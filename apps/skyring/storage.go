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
	storage_post_functions = map[string]string{
		"create": "CreateStorage",
		"expand": "ExpandStorage",
		"delete": "RemoveStorage",
		"update": "UpdateStorage",
	}

	STORAGE_STATUS_UP   = "up"
	STORAGE_STATUS_DOWN = "down"
)

func (a *App) POST_Storages(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s-Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error checking managed state of cluster: %v", *cluster_id))
		return
	}
	if ok {
		logger.Get().Error("%s-Cluster: %v is in un-managed state", ctxt, *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %v is in un-managed state", *cluster_id))
		return
	}

	var request models.AddStorageRequest
	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Check if storage entity already added
	// No need to check for error as storage would be nil in case of error and the same is checked
	if storage, _ := storage_exists("name", request.Name); storage != nil {
		logger.Get().Error("%s-Storage entity: %s already added", ctxt, request.Name)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Storage entity: %s already added", request.Name))
		return
	}

	// Validate storage type
	if ok := valid_storage_type(request.Type); !ok {
		logger.Get().Error("Invalid storage type: %s", request.Type)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid storage type: %s", request.Type))
		return
	}

	// Validate storage target size info
	if request.Size != "" {
		if ok, err := valid_storage_size(request.Size); !ok || err != nil {
			logger.Get().Error(
				"%s-Invalid storage size: %v",
				ctxt,
				request.Size)
			HttpResponse(
				w,
				http.StatusBadRequest,
				fmt.Sprintf(
					"Invalid storage size: %s passed for: %s",
					request.Size,
					request.Name))
			return
		}
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
				t.UpdateStatus("Started the task for pool creation: %v", t.ID)

				nodes, err := getClusterNodesById(cluster_id)
				if err != nil {
					util.FailTask("Failed to get nodes", fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "Manage_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)

				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s-Error getting provider for cluster: %v", ctxt, *cluster_id)), t)
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, storage_post_functions["create"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("Error creating storage: %s on cluster: %v", request.Name, *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("Error parsing provider task id while creating storage: %s for cluster: %v", request.Name, *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
						return
					}
					t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("Error adding sub task while creating storage: %s on cluster: %v", request.Name, *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
						return
					}

					// Check for provider task to complete and update the parent task
					done := false
					for {
						time.Sleep(2 * time.Second)
						sessionCopy := db.GetDatastore().Copy()
						defer sessionCopy.Close()
						coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
						var providerTask models.AppTask
						if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
							util.FailTask(fmt.Sprintf("Error getting sub task status while creating storage: %s on cluster: %v", request.Name, *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
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
							done = true
							break
						}
					}
					if !done {
						util.FailTask(
							"Sub task timed out",
							fmt.Errorf("%s-Could not get sub task status after 5 minutes", ctxt),
							t)
					}
					return
				}
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Create Storage: %s", request.Name),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create task for create storage:%s on cluster: %v. error: %v", ctxt, request.Name, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for create storage")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v for creating storage on cluster: %v", ctxt, taskId, request.Name, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func storage_exists(key string, value string) (*models.Storage, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storage models.Storage
	if err := collection.Find(bson.M{key: value}).One(&storage); err != nil {
		return nil, err
	} else {
		return &storage, nil
	}
}

func (a *App) GET_Storages(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	params := r.URL.Query()
	storage_type := params.Get("type")

	var filter bson.M = make(map[string]interface{})
	filter["clusterid"] = *cluster_id
	if storage_type != "" {
		filter["type"] = storage_type
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storages models.Storages
	if err := collection.Find(filter).All(&storages); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("%s-Error getting the storage list for cluster: %v. error: %v", ctxt, *cluster_id, err)
		return
	}
	if len(storages) == 0 {
		json.NewEncoder(w).Encode(models.Storages{})
		return
	}
	for i := range storages {
		if err := GetSluIds(&storages[i], ctxt); err != nil {
			HttpResponse(w, http.StatusInternalServerError, err.Error())
			logger.Get().Error("%s-Error getting SLUs with given storage profile: %s. error: %v", ctxt, storages[i].Profile, err)
			return
		}
	}

	json.NewEncoder(w).Encode(storages)

}

func (a *App) GET_Storage(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}
	storage_id_str := vars["storage-id"]
	storage_id, err := uuid.Parse(storage_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the storage id: %s. error: %v", ctxt, storage_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the storage id: %s", storage_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storage models.Storage
	if err := collection.Find(bson.M{"clusterid": *cluster_id, "storageid": *storage_id}).One(&storage); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("%s-Error getting the storage: %v on cluster: %v. error: %v", ctxt, *storage_id, *cluster_id, err)
		return
	}
	if storage.Name == "" {
		HttpResponse(w, http.StatusBadRequest, "Storage not found")
		logger.Get().Error("%s-Storage with id: %v not found for cluster: %v. error: %v", ctxt, *storage_id, *cluster_id, err)
		return
	}
	if err := GetSluIds(&storage, ctxt); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("%s-Error getting SLUs with given storage profile: %s. error: %v", ctxt, storage.Profile, err)
		return
	}
	json.NewEncoder(w).Encode(storage)
}

func GetSluIds(storage *models.Storage, ctxt string) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	var slus []models.StorageLogicalUnit
	if err := collection.Find(bson.M{"storageprofile": storage.Profile}).All(&slus); err != nil {
		logger.Get().Error("%s-Error getting SLUs with given storage profile: %s. error: %v", ctxt, storage.Profile, err)
		return err
	}
	for _, slu := range slus {
		storage.SluIds = append(storage.SluIds, slu.SluId)
	}
	return nil
}

func (a *App) GET_AllStorages(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storages models.Storages
	if err := collection.Find(nil).All(&storages); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("%s-Error getting the storage list. error: %v", ctxt, err)
		return
	}
	if len(storages) == 0 {
		json.NewEncoder(w).Encode(models.Storages{})
		return
	}

	for i := range storages {
		if err := GetSluIds(&storages[i], ctxt); err != nil {
			HttpResponse(w, http.StatusInternalServerError, err.Error())
			logger.Get().Error("%s-Error getting SLUs with given storage profile: %s. error: %v", ctxt, storages[i].Profile, err)
			return
		}
	}
	json.NewEncoder(w).Encode(storages)
}

func (a *App) DEL_Storage(w http.ResponseWriter, r *http.Request) {
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

	// Check if block devices are backed by this storage
	// If so dont allow deletion and ask to delete block devices first
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_BLOCK_DEVICES)
	var blkDevices []models.BlockDevice
	err = coll.Find(bson.M{"clusterid": *cluster_id, "storageid": *storage_id}).All(&blkDevices)
	if err != nil {
		logger.Get().Error("%s-Error checking block devices backed by storage: %v", ctxt, *storage_id)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error checking block devices backed by storage: %v", *storage_id))
		return
	}
	if len(blkDevices) > 0 {
		logger.Get().Warning("%s-There are block devices backed by storage: %v. First block devices should be deleted.", ctxt, *storage_id)
		HttpResponse(
			w,
			http.StatusMethodNotAllowed,
			"There are block devices backed by storage. Make sure all connected clients are disconnected from block devices and first delete the block devices")
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
				t.UpdateStatus("Started the task for storage deletion: %v", t.ID)
				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					util.FailTask(fmt.Sprintf("%s - ", ctxt), errors.New(fmt.Sprintf("%s-Error getting provider for cluster: %v", ctxt, *cluster_id)), t)
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, storage_post_functions["delete"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s - Error deleting storage: %v", ctxt, *storage_id), err, t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("%s - Error parsing provider task id while deleting storage: %v", ctxt, *storage_id), err, t)
						return
					}
					t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("%s - Error adding sub task while deleting storage: %v", ctxt, *storage_id), err, t)
						return
					}

					// Check for provider task to complete and update the disk info
					sessionCopy := db.GetDatastore().Copy()
					defer sessionCopy.Close()
					coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
					var providerTask models.AppTask
					for {
						time.Sleep(2 * time.Second)
						if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
							util.FailTask(fmt.Sprintf("%s - Error getting sub task status while deleting storage: %v", ctxt, *storage_id), err, t)
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								t.UpdateStatus("Success")
								t.Done(models.TASK_STATUS_SUCCESS)
							} else {
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
		fmt.Sprintf("Delete Storage: %s", cluster_id_str),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s - Unable to create task to delete storage: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for delete storage")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v to delete storage: %v", ctxt, taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) PATCH_Storage(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error(
			"Error Getting the context. error: %v",
			err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error(
			"%s - Error parsing the cluster id: %s. error: %v",
			ctxt,
			cluster_id_str,
			err)
		HttpResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf(
				"Error parsing the cluster id: %s",
				cluster_id_str))
		return
	}
	storage_id_str := vars["storage-id"]
	storage_id, err := uuid.Parse(storage_id_str)
	if err != nil {
		logger.Get().Error(
			"%s - Error parsing the storage id: %s. error: %v",
			ctxt,
			storage_id_str,
			err)
		HttpResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf(
				"Error parsing the storage id: %s",
				storage_id_str))
		return
	}
	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error(
			"%s-Error checking managed state of cluster: %v. error: %v",
			ctxt,
			*cluster_id,
			err)
		HttpResponse(
			w,
			http.StatusMethodNotAllowed,
			fmt.Sprintf(
				"Error checking managed state of cluster: %v",
				*cluster_id))
		return
	}
	if ok {
		logger.Get().Error(
			"%s-Cluster: %v is in un-managed state",
			ctxt,
			*cluster_id)
		HttpResponse(
			w,
			http.StatusMethodNotAllowed,
			fmt.Sprintf(
				"Cluster: %v is in un-managed state",
				*cluster_id))
		return
	}

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error(
			"%s-Error parsing the request. error: %v",
			ctxt,
			err)
		HttpResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf(
				"Unable to parse the request: %v",
				err))
		return
	}

	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	asyncTask := func(t *task.Task) {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for storage update: %v", t.ID)
				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					util.FailTask(
						fmt.Sprintf("Error getting the provider for cluster: %v", *cluster_id),
						fmt.Errorf("%s-%v", ctxt, err),
						t)
					return
				}
				err = provider.Client.Call(
					fmt.Sprintf(
						"%s.%s",
						provider.Name,
						storage_post_functions["update"]),
					models.RpcRequest{
						RpcRequestVars:    vars,
						RpcRequestData:    body,
						RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(
						fmt.Sprintf(
							"Error updating storage: %v",
							*storage_id),
						fmt.Errorf("%s-%v", ctxt, err),
						t)
					return
				}
				// Update the master task id
				providerTaskId, err = uuid.Parse(result.Data.RequestId)
				if err != nil {
					util.FailTask(
						fmt.Sprintf(
							"Error parsing provider task id while updating storage: %v",
							*storage_id),
						err,
						t)
					return
				}
				t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
				if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
					util.FailTask(
						fmt.Sprintf(
							"Error adding sub task while updating storage: %v",
							*storage_id),
						err,
						t)
					return
				}

				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
				var providerTask models.AppTask
				for {
					time.Sleep(2 * time.Second)
					if err := coll.Find(
						bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
						util.FailTask(
							fmt.Sprintf(
								"Error getting sub task status while updating storage: %v",
								*storage_id),
							err,
							t)
						return
					}
					if providerTask.Completed {
						if providerTask.Status == models.TASK_STATUS_SUCCESS {
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)
						} else {
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
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Update Storage: %s", cluster_id_str),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s - Unable to create task to update storage: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for update storage")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v to update storage: %v", ctxt, taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}
