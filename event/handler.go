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
	"fmt"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/nodemanager/saltnodemanager"
	"github.com/skyrings/skyring/tools/logger"
	"gopkg.in/mgo.v2/bson"
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

func Persist_event(event models.Event) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_NODE_EVENTS)
	if err := coll.Insert(event); err != nil {
		logger.Get().Error("Error adding the node event: %v", err)
		return err
	}
	return nil
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
	if storage_node, ok := saltnodemanager.PopulateStorageNodeInstance(node); ok {
		//t.UpdateStatus("Applying the storage profiles: %s", request.Hostname)
		if err := skyring.ApplyStorageProfiles(storage_node); err != nil {
			logger.Get().Error(fmt.Sprintf("Error applying storage profiles %v", err))
		}
		//t.UpdateStatus("Adding the node to DB: %s", request.Hostname)
		if err := updateStorageNodeToDB(*storage_node); err != nil {
			logger.Get().Error("Unable to add the node: %s to DB. error: %v", node, err)
			//t.UpdateStatus("Unable to add the node: %s to DB. error: %v", node, err)
			return err
		}
		//t.UpdateStatus("Setting up collectd on node: %s", request.Hostname)
		if nodeErrorMap, configureError := skyring.GetCoreNodeManager().SetUpMonitoring(node, skyring.Curr_hostname); configureError != nil && len(nodeErrorMap) != 0 {
			//t.UpdateStatus("Unable to setup collectd on node: %s", request.Hostname)
			logger.Get().Error("Unable to setup collectd on %s because of %v", node, nodeErrorMap)
			if len(nodeErrorMap) != 0 {
				return fmt.Errorf("Unable to setup collectd on %s because of %v", node, nodeErrorMap)
			} else {
				return configureError
			}
		}
		return nil
	} else {
		if err := skyring.InitializeStorageNodeToDB(node, models.NODE_STATE_ACCEPT_FAILED, models.STATUS_UP); err != nil {
			logger.Get().Error("Unable to initialize the node:%s to DB. error: %v", node, err)
			//t.UpdateStatus("Unable to initialize the node:%s to DB. error: %v", node, err)
			return err
		}
	}
	return fmt.Errorf("Unable to handle start node event for node: %s", node)
}

func updateStorageNodeToDB(storage_node models.Node) error {
	// Add the node details to the DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)

	// Before persisting the node check if node with same node_id already exists
	// If so dont add the node to DB and reject the minion
	var node models.Node
	// No need to check for error as if node does not exist, that also returned as error
	// As long as node details populated, its a valid node existing
	_ = coll.Find(bson.M{"nodeid": storage_node.NodeId}).One(&node)
	if node.Hostname != "" {
		logger.Get().Critical(fmt.Sprintf("Node with id: %v already exists", storage_node.NodeId))
		return fmt.Errorf("Node with id: %v already exists", storage_node.NodeId)
	}
	storage_node.State = models.NODE_STATE_FREE
	// Persist the node details
	if err := coll.Update(bson.M{"hostname": storage_node.Hostname}, storage_node); err != nil {
		logger.Get().Critical("Error Updating the node: %s. error: %v", storage_node.Hostname, err)
		return err
	}
	return nil
}
