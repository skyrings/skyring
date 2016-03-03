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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring-common/utils"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/nodemanager/saltnodemanager"
	"github.com/skyrings/skyring/skyringutils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	curr_hostname, err = os.Hostname()
)

var EventType = map[string]string{
	"DRIVE_ADD":           "Drive Addition",
	"DRIVE_REMOVE":        "Drive Removal",
	"COLLECTD_STOPPED":    "Collectd Stopped",
	"COLLECTD_STARTED":    "Collectd Started",
	"NODE_LOST_CONTACT":   "Node contact lost",
	"NODE_GAINED_CONTACT": "Node contact gained",
	"MEMORY":              "Memory Threshold Crossed",
	"SWAP":                "Swap Threshold Crossed",
	"CPU":                 "Cpu Threshold Crossed",
	"DF":                  "Mount Threshold Crossed",
	"NETWORK_THRESHOLD_CROSSED": "Network Threshold Crossed",
}

var handlermap = map[string]interface{}{
	"skyring/dbus/node/*/generic/storage/drive/added":   drive_add_handler,
	"skyring/dbus/node/*/generic/storage/drive/removed": drive_remove_handler,
	"skyring/dbus/node/*/generic/service/collectd":      collectd_status_handler,
	"salt/node/appeared":                                node_appeared_handler,
	"salt/node/lost":                                    node_lost_handler,
	"skyring/collectd/node/*/threshold/memory/*":        resource_threshold_crossed,
	"skyring/collectd/node/*/threshold/swap/*":          resource_threshold_crossed,
	"skyring/collectd/node/*/threshold/cpu/*":           resource_threshold_crossed,
	"skyring/collectd/node/*/threshold/df/*":            resource_threshold_crossed,
}

func get_readable_float(str string, ctxt string) (string, error) {
	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		logger.Get().Error("%s-Could not parse the string: %s", ctxt, str)
		return str, err
	}
	return fmt.Sprintf("%.2f", f), nil
}

func resource_threshold_crossed(event models.AppEvent, ctxt string) (models.AppEvent, error) {
	event.Name = EventType[strings.ToUpper(event.Tags["Plugin"])]
	currentValue, currentValueErr := get_readable_float(event.Tags["CurrentValue"], ctxt)
	if currentValueErr != nil {
		logger.Get().Error("%s-Could not parse the CurrentValue: %s", ctxt, (event.Tags["Plugin"]))
		return event, currentValueErr
	}
	thresholdValue, thresholdValueErr := get_readable_float(event.Tags["FailureMax"], ctxt)
	if thresholdValueErr != nil {
		logger.Get().Error("%s-Could not parse the Failure max value: %s", ctxt, (event.Tags["FailureMax"]))
		return event, thresholdValueErr
	}
	if event.Tags["Plugin"] == "df" {
		event.Tags["Plugin"] = fmt.Sprintf("%s MountPoint", event.Tags["PluginInstance"])
	}
	if event.Tags["Severity"] == "FAILURE" {
		event.Description = fmt.Sprintf("%s utilization on the node: %s has crossed the threshold value of %s%%. Current utilization: %s%%", event.Tags["Plugin"], event.NodeName, thresholdValue, currentValue)
		event.Message = fmt.Sprintf("%s utilization crossed threshold on: %s", event.Tags["Plugin"], event.NodeName)
		event.EntityId = event.NodeId
		event.Severity = models.ALARM_STATUS_MAJOR
	} else if event.Tags["Severity"] == "OKAY" {
		event.Description = fmt.Sprintf("%s utilization on the node: %s is back to normal. Threshold value: %s%%. Current utilization: %s%%", event.Tags["Plugin"], event.NodeName, thresholdValue, currentValue)
		event.Message = fmt.Sprintf("%s utilization back to normal on: %s", event.Tags["Plugin"], event.NodeName)
		event.EntityId = event.NodeId
		event.Severity = models.ALARM_STATUS_CLEARED
	}
	event.Tags = map[string]string{
		"Current Utilization": currentValue,
		"Threshold value":     thresholdValue,
	}
	event.NotificationEntity = models.NOTIFICATION_ENTITY_HOST
	event.Notify = true
	return event, nil
}

func drive_add_handler(event models.AppEvent, ctxt string) (models.AppEvent, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"nodeid": event.NodeId}).One(&node); err != nil {
		logger.Get().Error("%s-Node information read from DB failed for node: %s. error: %v", ctxt, event.NodeId, err)
		return event, err
	}
	if node.State != models.NODE_STATE_ACTIVE {
		return event, nil
	}
	existing_disks := node.StorageDisks
	sProfiles, err := skyring.GetDbProvider().StorageProfileInterface().StorageProfiles(ctxt, nil, models.QueryOps{})
	if err != nil {
		logger.Get().Error("%s-Unable to get the storage profiles. err:%v", ctxt, err)
		return event, err
	}

	// sync the nodes to get new disks
	if ok, err := skyring.GetCoreNodeManager().SyncStorageDisks(node.Hostname, sProfiles, ctxt); err != nil || !ok {
		logger.Get().Error("%s-Failed to sync disk for host: %s Error: %v", ctxt, node.Hostname, err)
		return event, err
	}

	// get the list of disks after syncing.
	if err := coll.Find(bson.M{"nodeid": event.NodeId}).One(&node); err != nil {
		logger.Get().Error("%s-Node information read from DB failed for node: %s. error: %v", ctxt, event.NodeId, err)
		return event, err
	}
	var cluster_nodes []models.ClusterNode
	var cluster_node models.ClusterNode
	cluster_node.NodeId = event.NodeId.String()
	cluster_node.NodeType = []string{models.NODE_TYPE_OSD}
	var exists bool
	for _, disk := range node.StorageDisks {
		exists = false
		for _, existing_disk := range existing_disks {
			if disk.DevName == existing_disk.DevName {
				exists = true
				break
			}
		}
		if !exists && !disk.Used && disk.Type == "disk" {
			cluster_node.Devices = append(cluster_node.Devices, models.ClusterNodeDevice{Name: disk.DevName, FSType: "xfs"})
			event.EntityId = disk.DiskId
			event.Tags["DevName"] = disk.DevName
			event.Tags["size"] = strconv.FormatUint(disk.Size, 10)
			event.Tags["Type"] = disk.Type
		}
	}

	// adding the details for event

	event.Name = EventType["DRIVE_ADD"]
	event.NotificationEntity = models.NOTIFICATION_ENTITY_HOST
	event.Message = fmt.Sprintf("New Storage Drive: %s added to Host:%s", event.Tags["DevName"], node.Hostname)
	event.Severity = models.ALARM_STATUS_CLEARED
	event.Notify = false

	c := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := c.Find(bson.M{"clusterid": event.ClusterId}).One(&cluster); err != nil {
		logger.Get().Error("%s-Cluster information read from DB failed for Cluster: %s. error: %v", ctxt, event.ClusterId, err)
		return event, nil
	}

	// check if cluster is in managed/un-managed state
	ok, err := skyring.ClusterUnmanaged(event.ClusterId)
	if err != nil {
		logger.Get().Warning("%s-Error checking managed state of cluster: %v. error: %v", ctxt, event.ClusterId, err)
		return event, err
	}
	if ok {
		logger.Get().Error("%s-Cluster: %v is in un-managed state", ctxt, event.ClusterId)
		return event, err
	}

	vars := map[string]string{"cluster-id": event.ClusterId.String()}
	cluster_nodes = append(cluster_nodes, cluster_node)
	body, err := json.Marshal(cluster_nodes)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error forming request body. error: %v", ctxt, err))
		return event, nil
	}

	// lock the node for expanding

	var nodes models.Nodes
	if err := coll.Find(bson.M{"clusterid": event.ClusterId}).All(&nodes); err != nil {
		logger.Get().Error("%s-Node information read from DB failed . error: %v", ctxt, err)
		return event, nil
	}

	// check if autoExpand is enabled or not
	if !cluster.AutoExpand {
		return event, nil
	}

	// Expand cluster
	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started task for cluster expansion: %v", t.ID)
				appLock, err := skyring.LockNodes(ctxt, nodes, "Expand_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				defer skyring.GetApp().GetLockManager().ReleaseLock(ctxt, *appLock)

				provider := skyring.GetApp().GetProviderFromClusterId(ctxt, node.ClusterId)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s-Error etting provider for cluster: %v", ctxt, event.ClusterId)), t)
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, "ExpandCluster"),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("Error expanding cluster: %v", event.ClusterId), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				// Update the master task id
				providerTaskId, err = uuid.Parse(result.Data.RequestId)
				if err != nil {
					util.FailTask(fmt.Sprintf("Error parsing provider task id while expand cluster: %v", event.ClusterId), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				t.UpdateStatus("Adding sub task")
				if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
					util.FailTask(fmt.Sprintf("Error adding sub task while expand cluster: %v", event.ClusterId), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}

				// Check for provider task to complete and update the disk info
				for {
					time.Sleep(2 * time.Second)
					sessionCopy := db.GetDatastore().Copy()
					defer sessionCopy.Close()
					coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
					var providerTask models.AppTask
					if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
						util.FailTask(fmt.Sprintf("Error getting sub task status while expand cluster: %v", event.ClusterId), fmt.Errorf("%s-%v", ctxt, err), t)
						return
					}
					if providerTask.Completed {
						if providerTask.Status == models.TASK_STATUS_SUCCESS {
							t.UpdateStatus("Starting disk sync")
							if ok, err := skyring.GetCoreNodeManager().SyncStorageDisks(node.Hostname, sProfiles, ctxt); err != nil || !ok {
								logger.Get().Error("%s-Failed to sync disk for host: %s Error: %v", ctxt, node.Hostname, err)
								return
							}
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)

						} else {
							logger.Get().Error("%s-Failed to expand the cluster %s", ctxt, event.ClusterId)
							t.UpdateStatus("Failed")
							t.Done(models.TASK_STATUS_FAILURE)
						}
						break
					}
				}
				return

			}
		}
	}

	if taskId, err := skyring.GetApp().GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Expand Cluster: %s", event.ClusterId.String()),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create task to expand cluster: %v. error: %v", ctxt, event.ClusterId, err)
		return event, nil
	} else {
		logger.Get().Debug("%s-Task Created: %v to expand cluster: %v", ctxt, taskId, event.ClusterId)
		return event, nil
	}

}

func drive_remove_handler(event models.AppEvent, ctxt string) (models.AppEvent, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"nodeid": event.NodeId}).One(&node); err != nil {
		logger.Get().Error("%s-Node information read from DB failed for node: %s. error: %v", ctxt, event.NodeId, err)
		return event, err
	}

	sProfiles, err := skyring.GetDbProvider().StorageProfileInterface().StorageProfiles(ctxt, nil, models.QueryOps{})
	if err != nil {
		logger.Get().Error("%s-Unable to get the storage profiles. err:%v", ctxt, err)
		return event, err
	}

	previous_disks := node.StorageDisks
	// sync the nodes to get new disks
	if ok, err := skyring.GetCoreNodeManager().SyncStorageDisks(node.Hostname, sProfiles, ctxt); err != nil || !ok {
		logger.Get().Error("%s-Failed to sync disk for host: %s Error: %v", ctxt, node.Hostname, err)
		return event, err
	}

	if err := coll.Find(bson.M{"nodeid": event.NodeId}).One(&node); err != nil {
		logger.Get().Error("%s-Node information read from DB failed for node: %s. error: %v", ctxt, event.NodeId, err)
		return event, err
	}

	var exists bool
	for _, disk := range previous_disks {
		exists = false
		for _, new_disk := range node.StorageDisks {
			if disk.DevName == new_disk.DevName {
				exists = true
				break
			}
		}
		if !exists && !disk.Used {
			event.EntityId = disk.DiskId
			event.Tags["DevName"] = disk.DevName
			event.Tags["DriveSize"] = strconv.FormatUint(disk.Size, 10)
			event.Tags["Type"] = disk.Type

		}
	}

	// adding the details for event

	event.Name = EventType["DRIVE_REMOVE"]
	event.NotificationEntity = models.NOTIFICATION_ENTITY_HOST
	event.Message = fmt.Sprintf("Storage Drive: %s removed from Host:%s", event.Tags["DevName"], node.Hostname)
	event.Severity = models.ALARM_STATUS_CLEARED
	event.Notify = false
	return event, nil
}

func collectd_status_handler(event models.AppEvent, ctxt string) (models.AppEvent, error) {
	// adding the details for event
	if strings.HasSuffix(event.Message, "inactive") {
		event.Name = EventType["COLLECTD_STOPPED"]
		event.Message = fmt.Sprintf("Collectd process stopped on Host: %s", event.NodeName)
		event.Description = fmt.Sprintf("Collectd process is stopped on Host: %s. This might affect the monitoring functionality of skyring", event.NodeName)
		event.EntityId = event.NodeId
		event.Severity = models.ALARM_STATUS_MAJOR
	} else if strings.HasSuffix(event.Message, "active") {
		event.Name = EventType["COLLECTD_STARTED"]
		event.Message = fmt.Sprintf("Collectd process started on Host: %s", event.NodeName)
		event.EntityId = event.NodeId
		event.Severity = models.ALARM_STATUS_CLEARED
	} else {
		return event, nil
	}
	event.NotificationEntity = models.NOTIFICATION_ENTITY_HOST
	event.Notify = true
	return event, nil
}

func node_appeared_handler(event models.AppEvent, ctxt string) (models.AppEvent, error) {
	//worry about the node only if the node in active state
	state, err := skyringutils.GetNodeStateById(ctxt, event.NodeId)
	if state == models.NODE_STATE_ACTIVE && err == nil {
		if err := skyringutils.Update_node_status_byId(ctxt, event.NodeId, models.NODE_STATUS_OK); err != nil {
			return event, err
		}
	}
	event.Name = EventType["NODE_GAINED_CONTACT"]
	event.Message = fmt.Sprintf("Host: %s gained contact", event.NodeName)
	event.EntityId = event.NodeId
	event.Severity = models.ALARM_STATUS_CLEARED
	event.NotificationEntity = models.NOTIFICATION_ENTITY_HOST
	event.Notify = true
	return event, nil
}

func node_lost_handler(event models.AppEvent, ctxt string) (models.AppEvent, error) {
	//worry about the node only if the node in active state
	state, err := skyringutils.GetNodeStateById(ctxt, event.NodeId)
	if state == models.NODE_STATE_ACTIVE && err == nil {
		if err := skyringutils.Update_node_status_byId(ctxt, event.NodeId, models.NODE_STATUS_ERROR); err != nil {
			return event, err
		}
	}
	event.Name = EventType["NODE_LOST_CONTACT"]
	event.Message = fmt.Sprintf("Host: %s lost contact", event.NodeName)
	event.EntityId = event.NodeId
	event.Severity = models.ALARM_STATUS_INDETERMINATE
	event.NotificationEntity = models.NOTIFICATION_ENTITY_HOST
	event.Notify = true
	return event, nil
}

func collectd_threshold_handler(event models.AppEvent, ctxt string) (models.AppEvent, error) {
	return event, nil
}

func handle_node_start_event(node string, ctxt string) error {
	skyring.Initialize(node, ctxt)
	return nil
}

func handle_UnManagedNode(hostname string, ctxt string) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	err := coll.Find(bson.M{"hostname": hostname}).One(&node)
	if err == mgo.ErrNotFound {
		node.Hostname = hostname
		node.State = models.NODE_STATE_UNACCEPTED
		if node.Fingerprint, err = saltnodemanager.GetFingerPrint(node.Hostname, ctxt); err != nil {
			logger.Get().Error(fmt.Sprintf("%s-Faild to retrive fingerprint from : %s", ctxt, node.Hostname))
			return err
		}
		if err := coll.Insert(node); err != nil {
			logger.Get().Error(fmt.Sprintf("%s-Error adding Unmanaged node : %s. error: %v", ctxt, node.Hostname, err))
			return err
		}
		return nil
	}
	return errors.New(fmt.Sprintf("Node with hostname: %v already exists", hostname))
}
