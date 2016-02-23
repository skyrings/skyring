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
	}
)

func (a *App) POST_BlockDevicesWithStorage(w http.ResponseWriter, r *http.Request) {
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
	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s - Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error checking managed state of cluster: %v", *cluster_id))
		return
	}
	if ok {
		logger.Get().Error("%s - Cluster: %v is in un-managed state", ctxt, *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %v is in un-managed state", *cluster_id))
		return
	}

	var request models.AddClusterBlockDeviceRequest
	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s - Error parsing the request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s - Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Check if block device already added
	// No need to check for error as storage would be nil in case of error and the same is checked
	if blkDevice, _ := block_device_exists("name", request.Name); blkDevice != nil {
		logger.Get().Error("%s - Block device: %s already added", ctxt, request.Name)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Block device: %s already added", request.Name))
		return
	}

	// Validate storage target size info
	if ok, err := valid_storage_size(request.Size); !ok || err != nil {
		logger.Get().Error("%s - Invalid size: %v", ctxt, request.Size)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid size: %s passed for: %s", request.Size, request.Name))
		return
	}
	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	// Get the specific provider and invoke the method
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for block device creation: %v", t.ID)
				provider := a.getProviderFromClusterId(*cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s - Error getting provider for cluster: %v", ctxt, *cluster_id)), t)
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, block_device_functions["create"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s - Error creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("%s - Error parsing provider task id while creating block device: %s for cluster: %v", ctxt, request.Name, *cluster_id), err, t)
						return
					}
					t.UpdateStatus("Adding sub task")
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("%s - Error adding sub task while creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
						return
					}
					// Check for provider task to complete and update the parent task
					for {
						time.Sleep(2 * time.Second)
						sessionCopy := db.GetDatastore().Copy()
						defer sessionCopy.Close()
						coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
						var providerTask models.AppTask
						if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
							util.FailTask(fmt.Sprintf("%s - Error getting sub task status while creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								t.UpdateStatus("Success")
								t.Done(models.TASK_STATUS_SUCCESS)
							} else if providerTask.Status == models.TASK_STATUS_FAILURE {
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
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Create Block Device: %s", request.Name), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
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
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s - Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error checking managed state of cluster: %v", *cluster_id))
		return
	}
	if ok {
		logger.Get().Error("%s - Cluster: %v is in un-managed state", ctxt, *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %v is in un-managed state", *cluster_id))
		return
	}

	var request models.AddStorageBlockDeviceRequest
	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s - Error parsing the request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s - Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Check if block device already added
	// No need to check for error as storage would be nil in case of error and the same is checked
	if blkDevice, _ := block_device_exists("name", request.Name); blkDevice != nil {
		logger.Get().Error("%s - Block device: %s already added", ctxt, request.Name)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Block device: %s already added", request.Name))
		return
	}

	// Validate storage target size info
	if ok, err := valid_storage_size(request.Size); !ok || err != nil {
		logger.Get().Error("%s - Invalid size: %v", ctxt, request.Size)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid size: %s passed for: %s", request.Size, request.Name))
		return
	}
	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	// Get the specific provider and invoke the method
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for block device creation: %v", t.ID)
				provider := a.getProviderFromClusterId(*cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s - Error getting provider for cluster: %v", ctxt, *cluster_id)), t)
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, block_device_functions["create"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s - Error creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("%s - Error parsing provider task id while creating block device: %s for cluster: %v", ctxt, request.Name, *cluster_id), err, t)
						return
					}
					t.UpdateStatus("Adding sub task")
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("%s - Error adding sub task while creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
						return
					}
					// Check for provider task to complete and update the parent task
					for {
						time.Sleep(2 * time.Second)
						sessionCopy := db.GetDatastore().Copy()
						defer sessionCopy.Close()
						coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
						var providerTask models.AppTask
						if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
							util.FailTask(fmt.Sprintf("%s - Error getting sub task status while creating block device: %s on cluster: %v", ctxt, request.Name, *cluster_id), err, t)
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								t.UpdateStatus("Success")
								t.Done(models.TASK_STATUS_SUCCESS)
							} else if providerTask.Status == models.TASK_STATUS_FAILURE {
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
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Create Block Device: %s", request.Name), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
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
