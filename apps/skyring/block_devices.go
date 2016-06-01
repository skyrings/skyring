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
package skyring

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring-common/utils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

var (
	block_device_functions = map[string]string{
		"create": "CreateBlockDevice",
		"delete": "DeleteBlockDevice",
		"resize": "ResizeBlockDevice",
	}
)

// @Title POST_BlockDevices
// @Description Adds a block device to the cluster
// @Param cluster-id        path  string                         true  "UUID of the cluster"
// @Param storage-id        path  string                         true  "UUID of the storage entity using which block device to create"
// @Param name              form  string                         true  "name of the block device"
// @Param tags              form  models.StringArray             false "tags if any"
// @Param size              form  string                         true  "size of block device (in format <Num>MB/GB/TB/PB)"
// @Param snapshots_enabled form  bool                           false "whether snapshot enabled"
// @Param snapshot_schedule form  models.SnapshotScheduleRequest false "snapshot schedule details"
// @Param quota_enabled     form  bool                           true  "whether quota feature enabled"
// @Param quota_params      form  models.GenericMap              true  "storage specific quota parameters name:value"
// @Param options           form  models.GenericMap              false "name:value pair to set any other options"
// @Success 200 {object} string
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/clusters
// @router /api/v1/clusters/{cluster-id}/storages/{storage-id}/blockdevices [post]
func (a *App) POST_BlockDevices(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
			fmt.Sprintf("Failed to create block device for cluster: %s", cluster_id_str),
			fmt.Sprintf("Failed to create block device for cluster: %s Error: %v", cluster_id_str, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}
	clusterName, err := GetClusterNameById(cluster_id)
	if err != nil {
		clusterName = cluster_id_str
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s - Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
			fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
			fmt.Sprintf("Failed to create block device for cluster: %s Error: %v", clusterName, err),
			nil,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error checking managed state of cluster: %v", *cluster_id))
		return
	}
	if ok {
		logger.Get().Error("%s - Cluster: %v is in un-managed state", ctxt, *cluster_id)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
			fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
			fmt.Sprintf(
				"Failed to create block device for cluster: %s Error: %v",
				clusterName,
				fmt.Errorf("Cluster is un-managed")),
			nil,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %v is in un-managed state", *cluster_id))
		return
	}

	var request models.AddStorageBlockDeviceRequest
	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s - Error parsing the request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
			fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
			fmt.Sprintf("Failed to create block device for cluster: %s Error: %v", clusterName, err),
			nil,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s - Unable to unmarshal request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
			fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
			fmt.Sprintf("Failed to create block device for cluster: %s Error: %v", clusterName, err),
			nil,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Check if block device already added
	// No need to check for error as storage would be nil in case of error and the same is checked
	if blkDevice, _ := block_device_exists("name", request.Name); blkDevice != nil {
		logger.Get().Error("%s - Block device: %s already added", ctxt, request.Name)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
			fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
			fmt.Sprintf(
				"Failed to create block device for cluster: %s Error: %v",
				clusterName,
				fmt.Errorf("Block device exists")),
			nil,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Block device: %s already added", request.Name))
		return
	}

	// Validate storage target size info
	if ok, err := valid_size(request.Size); !ok || err != nil {
		logger.Get().Error("%s - Invalid size: %v", ctxt, request.Size)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
			fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
			fmt.Sprintf(
				"Failed to create block device for cluster: %s Error: %v",
				clusterName,
				fmt.Sprintf("Invalid size passed")),
			nil,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid size: %s passed for: %s", request.Size, request.Name))
		return
	}
	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	// Get the specific provider and invoke the method
	asyncTask := func(t *task.Task) {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for block device creation: %v", t.ID)
				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s - Error getting provider for cluster: %v", ctxt, *cluster_id)), t)
					if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
						fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
						fmt.Sprintf("Failed to create block device for cluster: %s Error: %v", clusterName,
							fmt.Errorf("Error getting provider")),
						nil,
						cluster_id,
						models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
					}
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, block_device_functions["create"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s - Error creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
					if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
						fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
						fmt.Sprintf("Failed to create block device for cluster: %s Error: %v", clusterName, err),
						nil,
						cluster_id,
						models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
					}
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("%s - Error parsing provider task id while creating block device: %s for cluster: %v", ctxt, request.Name, *cluster_id), err, t)
						if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
							fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
							fmt.Sprintf("Failed to create block device for cluster: %s Error: %v", clusterName, err),
							nil,
							cluster_id,
							models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
						}
						return
					}
					t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("%s - Error adding sub task while creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
						if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
							fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
							fmt.Sprintf("Failed to create block device for cluster: %s Error: %v",
								clusterName,
								fmt.Errorf("Error while adding subtask")),
							nil,
							cluster_id,
							models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
						}
						return
					}
					// Check for provider task to complete and update the parent task
					coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
					var providerTask models.AppTask
					for {
						time.Sleep(2 * time.Second)
						if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
							util.FailTask(fmt.Sprintf("%s - Error getting sub task status while creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
							if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
								fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
								fmt.Sprintf("Failed to create block device for cluster: %s Error: %v", clusterName, err),
								nil,
								cluster_id,
								models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
							}
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
									fmt.Sprintf("Created block device for cluster: %s", clusterName),
									fmt.Sprintf("Created block device for cluster: %s", clusterName),
									nil,
									cluster_id,
									models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
									&(t.ID),
									ctxt); err != nil {
									logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
								}
								t.UpdateStatus("Success")
								t.Done(models.TASK_STATUS_SUCCESS)
							} else if providerTask.Status == models.TASK_STATUS_FAILURE {
								if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_CREATED"],
									fmt.Sprintf("Failed to create block device for cluster: %s", clusterName),
									fmt.Sprintf("Failed to create block device for cluster: %s Error: %v",
										clusterName, fmt.Errorf("Provider Task Failed")),
									nil,
									cluster_id,
									models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
									&(t.ID),
									ctxt); err != nil {
									logger.Get().Error("%s- Unable to log create block device for cluster event. Error: %v", ctxt, err)
								}
								t.UpdateStatus("Failed")
								t.Done(models.TASK_STATUS_FAILURE)
							}
							break
						}
					}
				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Create Block Device: %s", request.Name),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s - Unable to create task for create block device:%s on cluster: %v. error: %v", ctxt, request.Name, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for create block device")
		return
	} else {
		logger.Get().Debug("%s - Task Created: %v for creating block device on cluster: %v", ctxt, taskId, request.Name, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

// @Title GET_BlockDevices
// @Description Retrieves block devices from system based on criteria
// @Param clusterid  query string true  "UUID of the cluster"
// @Param storageid  query string true  "UUID of the storage entity using which block device to create"
// @Success 200 {object} models.BlockDevices
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/blockdevices
// @router /api/v1/blockdevices [get]
func (a *App) GET_BlockDevices(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error getting the context. error: %v", err)
	}

	var filter bson.M = make(map[string]interface{})
	params := r.URL.Query()
	cluster_id_str := params.Get("clusterid")
	if cluster_id_str != "" {
		filter["clusterid"], err = uuid.Parse(cluster_id_str)
		if err != nil {
			logger.Get().Error("%s - Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
			HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
			return
		}
	}
	storage_id_str := params.Get("storageid")
	if storage_id_str != "" {
		filter["storageid"], err = uuid.Parse(storage_id_str)
		if err != nil {
			logger.Get().Error("%s - Error parsing the storage id: %s. error: %v", ctxt, storage_id_str, err)
			HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the storage id: %s", storage_id_str))
			return
		}
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_BLOCK_DEVICES)
	var blkDevices []models.BlockDevice
	if err := coll.Find(filter).All(&blkDevices); err != nil || len(blkDevices) == 0 {
		json.NewEncoder(w).Encode([]models.BlockDevice{})
	} else {
		json.NewEncoder(w).Encode(blkDevices)
	}
}

// @Title GET_ClusterBlockDevices
// @Description Retrieves block devices for a given cluster
// @Param cluster-id  path  string true  "UUID of the cluster"
// @Success 200 {object} models.BlockDevices
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/clusters
// @router /api/v1/clusters/{cluster-id}/blockdevices [get]
func (a *App) GET_ClusterBlockDevices(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_BLOCK_DEVICES)
	var blkDevices []models.BlockDevice
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).All(&blkDevices); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("%s - Error getting the block devices list for cluster: %v. error: %v", ctxt, *cluster_id, err)
		return
	}
	if len(blkDevices) == 0 {
		json.NewEncoder(w).Encode([]models.BlockDevice{})
	} else {
		json.NewEncoder(w).Encode(blkDevices)
	}
}

// @Title GET_ClusterStorageBlockDevices
// @Description Retrieves block devices for a given cluster and storage entity
// @Param cluster-id  path  string true  "UUID of the cluster"
// @Param storage-id  path  string true  "UUID of the storage entity"
// @Success 200 {object} models.BlockDevices
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/clusters
// @router /api/v1/clusters/{cluster-id}/storages/{storage-id}/blockdevices [get]
func (a *App) GET_ClusterStorageBlockDevices(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}
	storage_id_str := vars["storage-id"]
	storage_id, err := uuid.Parse(storage_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the storage id: %s. error: %v", ctxt, storage_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the storage id: %s", storage_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_BLOCK_DEVICES)
	var blkDevices []models.BlockDevice
	if err := collection.Find(bson.M{"clusterid": *cluster_id, "storageid": *storage_id}).All(&blkDevices); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("%s - Error getting the block devices list for storage %v of cluster: %v. error: %v", ctxt, *storage_id, *cluster_id, err)
		return
	}
	if len(blkDevices) == 0 {
		json.NewEncoder(w).Encode([]models.BlockDevice{})
	} else {
		json.NewEncoder(w).Encode(blkDevices)
	}
}

// @Title GET_BlockDevice
// @Description Retrieves a specific block device from system
// @Param cluster-id     path  string true  "UUID of the cluster"
// @Param storage-id     path  string true  "UUID of the storage entity"
// @Param blockdevice-id path  string true  "UUID of the block device"
// @Success 200 {object} models.BlockDevice
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/clusters
// @router /api/v1/clusters/{cluster-id}/storages/{storage-id}/blockdevices/{blockdevice-id} [get]
func (a *App) GET_BlockDevice(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}
	storage_id_str := vars["storage-id"]
	storage_id, err := uuid.Parse(storage_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the storage id: %s. error: %v", ctxt, storage_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the storage id: %s", storage_id_str))
		return
	}
	blockdevice_id_str := vars["blockdevice-id"]
	blockdevice_id, err := uuid.Parse(blockdevice_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the block device id: %s. error: %v", ctxt, blockdevice_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the block device id: %s", blockdevice_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_BLOCK_DEVICES)
	var blkDevice models.BlockDevice
	if err := collection.Find(bson.M{"clusterid": *cluster_id, "storageid": *storage_id, "id": *blockdevice_id}).One(&blkDevice); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("%s - Error getting the block device %v of storage %v on cluster: %v. error: %v", ctxt, *blockdevice_id, *storage_id, *cluster_id, err)
		return
	}
	if blkDevice.Name == "" {
		HttpResponse(w, http.StatusBadRequest, "Block device not found")
		logger.Get().Error("%s - Block device with id: %v not found for storage %v on cluster: %v. error: %v", ctxt, *blockdevice_id, *storage_id, *cluster_id, err)
		return
	} else {
		json.NewEncoder(w).Encode(blkDevice)
	}
}

// @Title DELETE_BlockDevice
// @Description Removes a specific block device from system
// @Param cluster-id     path  string true  "UUID of the cluster"
// @Param blockdevice-id path  string true  "UUID of the block device"
// @Success 200 {object} string
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/clusters
// @router /api/v1/clusters/{cluster-id}/storages/{storage-id}/blockdevices/{blockdevice-id} [delete]
func (a *App) DELETE_BlockDevice(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
			fmt.Sprintf("Failed to delete block device for cluster: %s", cluster_id_str),
			fmt.Sprintf("Failed to delete block device for cluster: %s Error: %v", cluster_id_str, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}
	clusterName, err := GetClusterNameById(cluster_id)
	if err != nil {
		clusterName = cluster_id_str
	}

	blockdevice_id_str := vars["blockdevice-id"]
	blockdevice_id, err := uuid.Parse(blockdevice_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the block device id: %s. error: %v", ctxt, blockdevice_id_str, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
			fmt.Sprintf("Failed to delete block device:%s for cluster: %s", blockdevice_id_str, clusterName),
			fmt.Sprintf("Failed to delete block device:%s for cluster: %s Error: %v", blockdevice_id_str, clusterName, err),
			nil,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the block device id: %s", blockdevice_id_str))
		return
	}
	blkDevice, _ := block_device_exists("id", *blockdevice_id)
	if blkDevice == nil {
		logger.Get().Error("%s - Block device: %v does not exist", ctxt, *blockdevice_id)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
			fmt.Sprintf("Failed to delete block device:%s for cluster: %s", blockdevice_id_str, clusterName),
			fmt.Sprintf(
				"Failed to delete block device:%s for cluster: %s Error: %v",
				blockdevice_id_str,
				clusterName,
				fmt.Errorf("Block device does not exist")),
			blockdevice_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Block device: %v not found", *blockdevice_id))
		return
	}

	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	// Get the specific provider and invoke the method
	asyncTask := func(t *task.Task) {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for block device deletion: %v", t.ID)
				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s - Error getting provider for cluster: %v", ctxt, *cluster_id)), t)
					if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
						fmt.Sprintf("Failed to delete block device:%s for cluster: %s", blkDevice.Name, clusterName),
						fmt.Sprintf("Failed to delete block device:%s for cluster: %s Error: %v", blkDevice.Name,
							clusterName, fmt.Errorf("Error getting provider for this cluster")),
						blockdevice_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
					}
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, block_device_functions["delete"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s - Error deleting block device: %v on cluster: %v", ctxt, *blockdevice_id, *cluster_id), err, t)
					if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
						fmt.Sprintf("Failed to delete block device:%s for cluster: %s", blkDevice.Name, clusterName),
						fmt.Sprintf("Failed to delete block device:%s for cluster: %s Error: %v", blkDevice.Name, clusterName, err),
						blockdevice_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
					}
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("%s - Error parsing provider task id while deleting block device: %v for cluster: %v", ctxt, *blockdevice_id, *cluster_id), err, t)
						if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
							fmt.Sprintf("Failed to delete block device:%s for cluster: %s", blkDevice.Name, clusterName),
							fmt.Sprintf("Failed to delete block device:%s for cluster: %s Error: %v",
								blkDevice.Name,
								clusterName, err),
							blockdevice_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
						}
						return
					}
					t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("%s - Error adding sub task while deleting block device: %v on cluster: %v", ctxt, *blockdevice_id, *cluster_id), err, t)
						if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
							fmt.Sprintf("Failed to delete block device:%s for cluster: %s",
								blkDevice.Name, clusterName),
							fmt.Sprintf("Failed to delete block device:%s for cluster: %s Error: %v",
								blkDevice.Name, clusterName,
								fmt.Errorf("Error while adding subtask")),
							blockdevice_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
						}
						return
					}
					// Check for provider task to complete and update the parent task
					coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
					var providerTask models.AppTask
					for {
						time.Sleep(2 * time.Second)
						if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
							util.FailTask(fmt.Sprintf("%s - Error getting sub task status while deleting block device: %v on cluster: %v", ctxt, *blockdevice_id, *cluster_id), err, t)
							if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
								fmt.Sprintf("Failed to delete block device:%s for cluster: %s",
									blkDevice.Name, clusterName),
								fmt.Sprintf("Failed to delete block device:%s for cluster: %s Error: %v",
									blkDevice.Name, clusterName, err),
								blockdevice_id,
								cluster_id,
								models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
							}
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
									fmt.Sprintf("Deleted block device:%s for cluster: %s",
										blkDevice.Name, clusterName),
									fmt.Sprintf("Deleted block device:%s for cluster: %s",
										blkDevice.Name, clusterName),
									blockdevice_id,
									cluster_id,
									models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
									&(t.ID),
									ctxt); err != nil {
									logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
								}
								t.UpdateStatus("Success")
								t.Done(models.TASK_STATUS_SUCCESS)
							} else if providerTask.Status == models.TASK_STATUS_FAILURE {
								if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_REMOVED"],
									fmt.Sprintf("Failed to delete block device:%s for cluster: %s",
										blkDevice.Name, clusterName),
									fmt.Sprintf("Failed to delete block device:%s for cluster: %s Error: %v",
										blkDevice.Name,
										clusterName,
										fmt.Errorf("Task for block device removal failed")),
									blockdevice_id,
									cluster_id,
									models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
									&(t.ID),
									ctxt); err != nil {
									logger.Get().Error("%s- Unable to log delete block device for cluster event. Error: %v", ctxt, err)
								}
								t.UpdateStatus("Failed")
								t.Done(models.TASK_STATUS_FAILURE)
							}
							break
						}
					}
				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Delete Block Device: %v", *blockdevice_id),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s - Unable to create task for delete block device:%v on cluster: %v. error: %v", ctxt, *blockdevice_id, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for create block device")
		return
	} else {
		logger.Get().Debug("%s - Task Created: %v for deleting block device %v on cluster: %v", ctxt, taskId, *blockdevice_id, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

// @Title PATCH_ResizeBlockDevice
// @Description partially update the size of a specific block device
// @Param cluster-id     path  string true  "UUID of the cluster"
// @Param blockdevice-id path  string true  "UUID of the block device"
// @Param size           form  string true  "new size for the block device (format <Num>MB/GB/TB/PB)"
// @Success 200 {object} string
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/clusters
// @router /api/v1/clusters/{cluster-id}/storages/{storage-id}/blockdevices/{blockdevice-id} [patch]
func (a *App) PATCH_ResizeBlockDevice(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
			fmt.Sprintf("Failed to resize block device for cluster: %s", cluster_id_str),
			fmt.Sprintf("Failed to resize block device for cluster: %s Error: %v", cluster_id_str, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}
	clusterName, err := GetClusterNameById(cluster_id)
	if err != nil {
		clusterName = cluster_id_str
	}

	blockdevice_id_str := vars["blockdevice-id"]
	blockdevice_id, err := uuid.Parse(blockdevice_id_str)
	if err != nil {
		logger.Get().Error("%s - Error parsing the block device id: %s. error: %v", ctxt, blockdevice_id_str, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
			fmt.Sprintf("Failed to resize block device: %s for cluster: %s",
				blockdevice_id_str, clusterName),
			fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v",
				blockdevice_id_str, clusterName, err),
			nil,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the block device id: %s", blockdevice_id_str))
		return
	}
	blkDevice, _ := block_device_exists("id", *blockdevice_id)
	if blkDevice == nil {
		logger.Get().Error("%s - Block device: %v does not exist", ctxt, *blockdevice_id)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
			fmt.Sprintf("Failed to resize block device: %s for cluster: %s",
				blockdevice_id_str, clusterName),
			fmt.Sprintf(
				"Failed to resize block device: %s for cluster: %s Error: %v",
				blockdevice_id_str,
				cluster_id_str,
				fmt.Errorf("Block device does not exist")),
			blockdevice_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Block device: %v not found", *blockdevice_id))
		return
	}

	var request struct {
		Size string `json:"size"`
	}
	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s - Error parsing the request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
			fmt.Sprintf("Failed to resize block device: %s for cluster: %s", blkDevice.Name,
				clusterName),
			fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v",
				blkDevice.Name, clusterName, err),
			blockdevice_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s - Unable to unmarshal request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
			fmt.Sprintf("Failed to resize block device: %s for cluster: %s", blkDevice.Name, clusterName),
			fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v", blkDevice.Name, clusterName, err),
			blockdevice_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Validate storage target size info
	if ok, err := valid_size(request.Size); !ok || err != nil {
		logger.Get().Error("%s - Invalid size: %v", ctxt, request.Size)
		if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
			fmt.Sprintf("Failed to resize block device: %s for cluster: %s", blkDevice.Name, clusterName),
			fmt.Sprintf(
				"Failed to resize block device: %s for cluster: %s Error: %v",
				blkDevice.Name,
				clusterName,
				fmt.Errorf("Invalid size passed")),
			blockdevice_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid size: %s passed for: %v", request.Size, *blockdevice_id))
		return
	}

	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	// Get the specific provider and invoke the method
	asyncTask := func(t *task.Task) {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for block device resize: %v", t.ID)
				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s - Error getting provider for cluster: %v", ctxt, *cluster_id)), t)
					if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
						fmt.Sprintf("Failed to resize block device: %s for cluster: %s", blkDevice.Name, clusterName),
						fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v",
							blkDevice.Name,
							clusterName,
							fmt.Errorf("Error getting provider for this cluster")),
						blockdevice_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
					}
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, block_device_functions["resize"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s - Error resizing block device: %v on cluster: %v", ctxt, *blockdevice_id, *cluster_id), err, t)
					if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
						fmt.Sprintf("Failed to resize block device: %s for cluster: %s", blkDevice.Name, clusterName),
						fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v",
							blkDevice.Name, clusterName, err),
						blockdevice_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
					}
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("%s - Error parsing provider task id while resizing block device: %v for cluster: %v", ctxt, *blockdevice_id, *cluster_id), err, t)
						if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
							fmt.Sprintf("Failed to resize block device: %s for cluster: %s",
								blkDevice.Name, clusterName),
							fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v",
								blkDevice.Name, clusterName, err),
							blockdevice_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
						}
						return
					}
					t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("%s - Error adding sub task while resizing block device: %v on cluster: %v", ctxt, *blockdevice_id, *cluster_id), err, t)
						if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
							fmt.Sprintf("Failed to resize block device: %s for cluster: %s",
								blkDevice.Name, clusterName),
							fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v",
								blkDevice.Name, clusterName, err),
							blockdevice_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
						}
						return
					}
					// Check for provider task to complete and update the parent task
					coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
					var providerTask models.AppTask
					for {
						time.Sleep(2 * time.Second)
						if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
							util.FailTask(fmt.Sprintf("%s - Error getting sub task status while resizing block device: %v on cluster: %v", ctxt, *blockdevice_id, *cluster_id), err, t)
							if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
								fmt.Sprintf("Failed to resize block device: %s for cluster: %s",
									blkDevice.Name, clusterName),
								fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v",
									blkDevice.Name, clusterName, err),
								blockdevice_id,
								cluster_id,
								models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
							}
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
									fmt.Sprintf("Resized block device: %s for cluster: %s",
										blkDevice.Name, clusterName),
									fmt.Sprintf("Resized block device: %s for cluster: %s",
										blkDevice.Name, clusterName),
									blockdevice_id,
									cluster_id,
									models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
									&(t.ID),
									ctxt); err != nil {
									logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
								}
								t.UpdateStatus("Success")
								t.Done(models.TASK_STATUS_SUCCESS)
							} else if providerTask.Status == models.TASK_STATUS_FAILURE {
								if err := logAuditEvent(EventTypes["BLOCK_DEVICVE_RESIZE"],
									fmt.Sprintf("Failed to resize block device: %s for cluster: %s",
										blkDevice.Name, clusterName),
									fmt.Sprintf("Failed to resize block device: %s for cluster: %s Error: %v",
										blkDevice.Name,
										clusterName,
										fmt.Errorf("Task for block device recize failed")),
									blockdevice_id,
									cluster_id,
									models.NOTIFICATION_ENTITY_BLOCK_DEVICE,
									&(t.ID),
									ctxt); err != nil {
									logger.Get().Error("%s- Unable to log resize block device for cluster event. Error: %v", ctxt, err)
								}
								t.UpdateStatus("Failed")
								t.Done(models.TASK_STATUS_FAILURE)
							}
							break
						}
					}
				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Resize Block Device: %v", *blockdevice_id),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s - Unable to create task for resize block device:%v on cluster: %v. error: %v", ctxt, *blockdevice_id, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for resize block device")
		return
	} else {
		logger.Get().Debug("%s - Task Created: %v for resizing block device %v on cluster: %v", ctxt, taskId, *blockdevice_id, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func block_device_exists(key string, value interface{}) (*models.BlockDevice, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_BLOCK_DEVICES)
	var blkDevice models.BlockDevice
	if err := collection.Find(bson.M{key: value}).One(&blkDevice); err != nil {
		return nil, err
	} else {
		return &blkDevice, nil
	}
}
