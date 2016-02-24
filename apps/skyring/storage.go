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
	}

	STORAGE_STATUS_UP   = "up"
	STORAGE_STATUS_DOWN = "down"
)

func (a *App) POST_Storages(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("Error checking managed state of cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error checking managed state of cluster: %v", *cluster_id))
		return
	}
	if ok {
		logger.Get().Error("Cluster: %v is in un-managed state", *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %v is in un-managed state", *cluster_id))
		return
	}

	var request models.AddStorageRequest
	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request. error: %v", err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Unable to unmarshal request. error: %v", err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Check if storage entity already added
	// No need to check for error as storage would be nil in case of error and the same is checked
	if storage, _ := storage_exists("name", request.Name); storage != nil {
		logger.Get().Error("Storage entity: %s already added", request.Name)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Storage entity: %s already added", request.Name))
		return
	}

	// Validate storage target size info
	if ok, err := valid_storage_size(request.Size); !ok || err != nil {
		logger.Get().Error("Invalid storage size: %v", request.Size)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid storage size: %s passed for: %s", request.Size, request.Name))
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
				t.UpdateStatus("Started the task for pool creation: %v", t.ID)

				nodes, err := getClusterNodesById(cluster_id)
				if err != nil {
					util.FailTask("Failed to get nodes", err, t)
					return
				}
				appLock, err := LockNodes(nodes, "Manage_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)

				provider := a.GetProviderFromClusterId(*cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("Error getting provider for cluster: %v", *cluster_id)), t)
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, storage_post_functions["create"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("Error creating storage: %s on cluster: %v", request.Name, *cluster_id), err, t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("Error parsing provider task id while creating storage: %s for cluster: %v", request.Name, *cluster_id), err, t)
						return
					}
					t.UpdateStatus("Adding sub task")
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("Error adding sub task while creating storage: %s on cluster: %v", request.Name, *cluster_id), err, t)
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
							util.FailTask(fmt.Sprintf("Error getting sub task status while creating storage: %s on cluster: %v", request.Name, *cluster_id), err, t)
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
							errors.New("Could not get sub task status after 5 minutes"),
							t)
					}
					return
				}
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Create Storage: %s", request.Name), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for create storage:%s on cluster: %v. error: %v", request.Name, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for create storage")
		return
	} else {
		logger.Get().Debug("Task Created: %v for creating storage on cluster: %v", taskId, request.Name, *cluster_id)
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
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storages models.Storages
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).All(&storages); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the storage list for cluster: %v. error: %v", *cluster_id, err)
		return
	}
	if len(storages) == 0 {
		json.NewEncoder(w).Encode(models.Storages{})
	} else {
		json.NewEncoder(w).Encode(storages)
	}
}

func (a *App) GET_Storage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}
	storage_id_str := vars["storage-id"]
	storage_id, err := uuid.Parse(storage_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the storage id: %s. error: %v", storage_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the storage id: %s", storage_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storage models.Storage
	if err := collection.Find(bson.M{"clusterid": *cluster_id, "storageid": *storage_id}).One(&storage); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the storage: %v on cluster: %v. error: %v", *storage_id, *cluster_id, err)
		return
	}
	if storage.Name == "" {
		HttpResponse(w, http.StatusBadRequest, "Storage not found")
		logger.Get().Error("Storage with id: %v not found for cluster: %v. error: %v", *storage_id, *cluster_id, err)
		return
	} else {
		json.NewEncoder(w).Encode(storage)
	}
}

func (a *App) GET_AllStorages(w http.ResponseWriter, r *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storages models.Storages
	if err := collection.Find(nil).All(&storages); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the storage list. error: %v", err)
		return
	}
	if len(storages) == 0 {
		json.NewEncoder(w).Encode(models.Storages{})
	} else {
		json.NewEncoder(w).Encode(storages)
	}
}
