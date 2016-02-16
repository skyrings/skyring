/*Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package event

import (
	"errors"
	"fmt"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/notifier"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/nodemanager/saltnodemanager"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"os"
	"time"
)

var (
	curr_hostname, err = os.Hostname()
)

var handlermap = map[string]interface{}{
	"skyring/dbus/node/*/generic/storage/block/added":           block_add_handler,
	"skyring/dbus/node/*/generic/storage/block/removed":         block_remove_handler,
	"skyring/dbus/node/*/generic/storage/block/changed":         block_change_handler,
	"skyring/dbus/node/*/generic/storage/mount/changed":         mount_change_handler,
	"skyring/dbus/node/*/generic/storage/drive/added":           drive_add_handler,
	"skyring/dbus/node/*/generic/storage/drive/removed":         drive_remove_handler,
	"skyring/dbus/node/*/generic/storage/drive/possibleFailure": drive_remove_handler,
	"skyring/dbus/node/*/generic/service/collectd":              collectd_status_handler,
	"salt/node/appeared":                                        node_appeared_handler,
	"salt/node/lost":                                            node_lost_handler,
	"skyring/collectd/node/*/threshold/*/*":                     collectd_threshold_handler,
}

// ALL HANDLERS ARE JUST WRITING THE EVENTS TO DB. OTHER HANDLING AND CORRELATION
// IS TO BE DONE

func block_add_handler(event models.Event) error {
	return nil
}

func block_remove_handler(event models.Event) error {
	return nil
}

func block_change_handler(event models.Event) error {
	return nil
}

func mount_change_handler(event models.Event) error {
	return nil
}

func drive_add_handler(event models.Event) error {
	return nil
}

func drive_remove_handler(event models.Event) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"nodeid": event.NodeId}).One(&node); err != nil {
		logger.Get().Error("Node information read from DB failed for node: %s. error: %v", event.NodeId, err)
		return nil
	}
	subject := fmt.Sprintf("Warning: Storage Drive Removed from %s", node.Hostname)
	body := event.Message
	notifier.MailNotify(subject, body, skyring.GetDbProvider())
	return nil
}

func collectd_status_handler(event models.Event) error {
	return nil
}

func update_node_status(nodeStatus string, event models.Event) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"nodeid": event.NodeId}, bson.M{"$set": bson.M{"status": nodeStatus}}); err != nil {
		logger.Get().Error("Error updating the node status: %s", err)
		return err
	}
	return nil
}

func node_appeared_handler(event models.Event) error {
	if err := update_node_status(models.STATUS_UP, event); err != nil {
		return err
	}
	return nil
}

func node_lost_handler(event models.Event) error {
	if err := update_node_status(models.STATUS_DOWN, event); err != nil {
		return err
	}
	return nil
}

func collectd_threshold_handler(event models.Event) error {
	return nil
}

func handle_node_start_event(node string) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)

	var storage_node models.Node

	_ = coll.Find(bson.M{"hostname": node}).One(&storage_node)
	if storage_node.State != models.NODE_STATE_INITIALIZING {
		logger.Get().Warning(fmt.Sprintf("Node with name: %s not in activating state to update other details", node))
		return nil
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("started the task for InitializeNode: %s", t.ID)
				// Process the request
				if err := initializeStorageNode(node, t); err != nil {
					t.UpdateStatus("Failed")
					t.Done(models.TASK_STATUS_FAILURE)
				} else {
					t.UpdateStatus("Success")
					t.Done(models.TASK_STATUS_SUCCESS)
				}
				return
			}
		}
	}
	if taskId, err := skyring.GetApp().GetTaskManager().Run(fmt.Sprintf("Initialize Node: %s", node), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create the task for Initialize Node: %s. error: %v", node, err)
	} else {
		logger.Get().Debug("Task created for initialize node. Task id: %s", taskId.String())
	}
	return nil
}

func initializeStorageNode(node string, t *task.Task) error {
	sProfiles, err := skyring.GetDbProvider().StorageProfileInterface().StorageProfiles(nil, models.QueryOps{})
	if err != nil {
		logger.Get().Error("Unable to get the storage profiles. May not be able to apply storage profiles for node: %v err:%v", node, err)
	}
	if storage_node, ok := saltnodemanager.GetStorageNodeInstance(node, sProfiles); ok {
		if err := updateStorageNodeToDB(*storage_node); err != nil {
			logger.Get().Error("Unable to add details of node: %s to DB. error: %v", node, err)
			t.UpdateStatus("Unable to add details of node: %s to DB. error: %v", node, err)
			updateNodeState(node, models.NODE_STATE_FAILED)
			return err
		}
		if nodeErrorMap, configureError := skyring.GetCoreNodeManager().SetUpMonitoring(node, curr_hostname); configureError != nil && len(nodeErrorMap) != 0 {
			if len(nodeErrorMap) != 0 {
				logger.Get().Error("Unable to setup collectd on %s because of %v", node, nodeErrorMap)
				t.UpdateStatus("Unable to setup collectd on %s because of %v", node, nodeErrorMap)
				updateNodeState(node, models.NODE_STATE_FAILED)
				return err
			} else {
				logger.Get().Error("Config Error during monitoring setup for node:%s Error:%v", node, configureError)
				t.UpdateStatus("Config Error during monitoring setup for node:%s Error:%v", node, configureError)
				updateNodeState(node, models.NODE_STATE_FAILED)
				return err
			}
		}
		if ok, err := skyring.GetCoreNodeManager().SyncModules(node); !ok || err != nil {
			logger.Get().Error("Failed to sync modules on the node: %s. error: %v", node, err)
			t.UpdateStatus("Failed to sync modules")
			updateNodeState(node, models.NODE_STATE_FAILED)
			return err
		}
		return nil
	} else {
		logger.Get().Critical("Error getting the details for node: %s", node)
		t.UpdateStatus("Error getting the details for node: %s", node)
		updateNodeState(node, models.NODE_STATE_FAILED)
		return fmt.Errorf("Error getting the details for node: %s", node)
	}
}

func updateNodeState(node string, state int) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"state": state}}); err != nil {
		logger.Get().Critical("Error updating the node state for node: %s. error: %v", node, err)
		return err
	}
	return nil
}

func updateStorageNodeToDB(storage_node models.Node) error {
	// Add the node details to the DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	err := coll.Find(bson.M{"nodeid": storage_node.NodeId}).One(&node)
	if err == mgo.ErrNotFound {
		storage_node.State = models.NODE_STATE_ACTIVE
		if err := coll.Update(bson.M{"hostname": storage_node.Hostname}, storage_node); err != nil {
			logger.Get().Critical("Error Updating the node: %s. error: %v", storage_node.Hostname, err)
			return err
		}
		return nil
	} else {
		logger.Get().Critical(fmt.Sprintf("Node with id: %v already exists", storage_node.NodeId))
		return errors.New(fmt.Sprintf("Node with id: %v already exists", storage_node.NodeId))
	}
}
