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
	"github.com/skyrings/skyring-common/monitoring"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring-common/utils"
	"github.com/skyrings/skyring/skyringutils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"
)

var (
	cluster_post_functions = map[string]string{
		"create":         "CreateCluster",
		"expand_cluster": "ExpandCluster",
		"patch_slu":      "UpdateStorageLogicalUnitParams",
		"config":         "GetClusterConfig",
	}

	storage_types = map[string]string{
		"ceph":    "block",
		"gluster": "file",
	}
)

func (a *App) PATCH_Clusters(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	var request map[string]interface{}
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]
	clid, err := uuid.Parse(cluster_id)
	if err != nil {
		logger.Get().Error("%s-Could not parse cluster uuid: %s", ctxt, cluster_id)
		if err := logAuditEvent(EventTypes["CLUSTER_UPDATED"],
			fmt.Sprintf("Failed to update cluster: %s", cluster_id),
			fmt.Sprintf("Failed to update cluster: %s Error: %v", cluster_id, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("could not parse cluster Uuid: %s", cluster_id))
		return
	}
	clusterName, err := GetClusterNameById(clid)
	if err != nil {
		clusterName = cluster_id
	}

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_UPDATED"],
			fmt.Sprintf("Failed to update cluster: %s", clusterName),
			fmt.Sprintf("Failed to update cluster: %s Error: %v", clusterName, err),
			clid,
			clid,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_UPDATED"],
			fmt.Sprintf("Failed to update cluster: %s", clusterName),
			fmt.Sprintf("Failed to update cluster: %s Error: %v", clusterName, err),
			clid,
			clid,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}
	var disableautoexpand bool
	if val, ok := request["disableautoexpand"]; ok {
		disableautoexpand = val.(bool)
	} else {
		logger.Get().Error("%s-Insufficient details for updating cluster", ctxt)
		if err := logAuditEvent(EventTypes["CLUSTER_UPDATED"],
			fmt.Sprintf("Failed to update cluster: %s", clusterName),
			fmt.Sprintf("Failed to update cluster: %s Error: %v", clusterName,
				fmt.Errorf("Insufficient details for updating cluster")),
			clid,
			clid,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update cluster event. Error: %v", ctxt, err)
		}
		HandleHttpError(w, errors.New("Insufficient details for updating cluster"))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{"clusterid": *clid}).One(&cluster); err != nil {
		logger.Get().Error("%s-Cluster: %s does not exists", ctxt, cluster_id)
		if err := logAuditEvent(EventTypes["CLUSTER_UPDATED"],
			fmt.Sprintf("Failed to update cluster: %s", clusterName),
			fmt.Sprintf("Failed to update cluster: %s Error: %v", clusterName, err),
			clid,
			clid,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %s does not exists", cluster_id))
		return
	}

	if err := collection.Update(bson.M{"clusterid": *clid}, bson.M{"$set": bson.M{"autoexpand": !disableautoexpand}}); err != nil {
		logger.Get().Error("%s-Failed updating cluster. Error:%v", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_UPDATED"],
			fmt.Sprintf("Failed to update cluster: %s", clusterName),
			fmt.Sprintf("Failed to update cluster: %s Error: %v", clusterName, err),
			clid,
			clid,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update cluster event. Error: %v", ctxt, err)
		}
		HandleHttpError(w, errors.New(fmt.Sprintf("Failed updating cluster. Error:%v", err)))
		return
	}
	if err := logAuditEvent(
		EventTypes["CLUSTER_UPDATED"],
		fmt.Sprintf("Updated cluster: %s", cluster.Name),
		fmt.Sprintf("Updated cluster: %s", cluster.Name),
		clid,
		clid,
		models.NOTIFICATION_ENTITY_CLUSTER,
		nil,
		ctxt); err != nil {
		logger.Get().Error("%s- Unable to log update cluster event. Error: %v", ctxt, err)
	}
	return
}

func (a *App) POST_Clusters(w http.ResponseWriter, r *http.Request) {
	var request models.AddClusterRequest

	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
			fmt.Sprintf("Failed to create cluster: %s", request.Name),
			fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
			fmt.Sprintf("Failed to create cluster: %s", request.Name),
			fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	// Check if cluster already added
	// Node need for error check as cluster would be nil in case of error
	cluster, _ := cluster_exists("name", request.Name)
	if cluster != nil {
		logger.Get().Error("%s-Cluster: %s already exists", ctxt, request.Name)
		if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
			fmt.Sprintf("Failed to create cluster: %s", request.Name),
			fmt.Sprintf(
				"Failed to create cluster: %s Error: %v",
				request.Name,
				fmt.Errorf("Cluster already exists")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error(
				"%s- Unable to log create cluster event. Error: %v",
				ctxt,
				err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %s already exists", request.Name))
		return
	}

	// Check if provided disks already utilized
	if used, err := disks_used(request.Nodes); err != nil {
		logger.Get().Error("%s-Error checking used state of disks for nodes. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
			fmt.Sprintf("Failed to create cluster: %s", request.Name),
			fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking used state of disks for nodes")
		return
	} else if used {
		logger.Get().Error("%s-Provided disks are already used", ctxt)
		if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
			fmt.Sprintf("Failed to create cluster: %s", request.Name),
			fmt.Sprintf(
				"Failed to create cluster: %s Error: %v",
				request.Name,
				fmt.Errorf("Disks already used")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error(
				"%s- Unable to log create cluster event. Error: %v",
				ctxt,
				err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Provided disks are already used")
		return
	}

	// Check if valid journal size passed
	if request.JournalSize != "" {
		if ok, err := valid_size(request.JournalSize); !ok || err != nil {
			logger.Get().Error(
				"%s-Invalid journal size: %v",
				ctxt,
				request.JournalSize)
			HttpResponse(
				w,
				http.StatusBadRequest,
				fmt.Sprintf(
					"Invalid journal size: %s passed for: %s",
					request.JournalSize,
					request.Name))
			return
		}
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
				t.UpdateStatus("Started the task for cluster creation: %v", t.ID)

				nodes, err := getClusterNodesFromRequest(request.Nodes)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %s", request.Name), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
						fmt.Sprintf("Failed to create cluster: %s", request.Name),
						fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, err),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
					}
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "POST_Clusters")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
						fmt.Sprintf("Failed to create cluster: %s", request.Name),
						fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, err),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
					}
					return
				}

				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)

				// Get the specific provider and invoke the method
				provider := a.getProviderFromClusterType(request.Type)
				if provider == nil {
					util.FailTask(fmt.Sprintf("%s-", ctxt), errors.New(fmt.Sprintf("Error getting provider for cluster: %s", request.Name)), t)
					if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
						fmt.Sprintf("Failed to create cluster: %s", request.Name),
						fmt.Sprintf(
							"Failed to create cluster: %s Error: %v",
							request.Name,
							fmt.Errorf("Unable to find provider")),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error(
							"%s- Unable to log create cluster event. Error: %v",
							ctxt,
							err)
					}
					return
				}
				t.UpdateStatus("Installing packages")
				//Install the packages
				if failedNodes := provider.ProvisionerName.Install(ctxt, t, provider.Name, request.Nodes); len(failedNodes) > 0 {
					logger.Get().Error("%s-Package Installation failed for nodes:%v", ctxt, failedNodes)
					successful := removeFailedNodes(request.Nodes, failedNodes)
					if len(successful) > 0 {
						request.Nodes = successful
						body, err = json.Marshal(request)
						if err != nil {
							util.FailTask(fmt.Sprintf("%s-", ctxt), err, t)
							if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
								fmt.Sprintf("Failed to create cluster: %s", request.Name),
								fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, err),
								nil,
								nil,
								models.NOTIFICATION_ENTITY_CLUSTER,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
							}
							return
						}
					} else {
						util.FailTask(fmt.Sprintf("%s-", ctxt), errors.New(fmt.Sprintf("Package Installation Failed for all nodes for cluster: %s", request.Name)), t)
						if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
							fmt.Sprintf("Failed to create cluster: %s", request.Name),
							fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, fmt.Errorf("Package installation failed")),
							nil,
							nil,
							models.NOTIFICATION_ENTITY_CLUSTER,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
						}
						return
					}
				}
				t.UpdateStatus("Installing packages done. Starting Cluster Creation.")
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, cluster_post_functions["create"]),
					models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: body, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s-Error creating cluster: %s", ctxt, request.Name), err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
						fmt.Sprintf("Failed to create cluster: %s", request.Name),
						fmt.Sprintf(
							"Failed to create cluster: %s Error: %v",
							request.Name,
							fmt.Errorf("Provider task failed")),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error(
							"%s- Unable to log create cluster event. Error: %v",
							ctxt,
							err)
					}
					return
				}
				// Update the master task id
				providerTaskId, err = uuid.Parse(result.Data.RequestId)
				if err != nil {
					util.FailTask(fmt.Sprintf("%s-Error parsing provider task id while creating cluster: %s", ctxt, request.Name), err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
						fmt.Sprintf("Failed to create cluster: %s", request.Name),
						fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, err),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
					}
					return
				}
				t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
				if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
					util.FailTask(fmt.Sprintf("%s-Error adding sub task while creating cluster: %s", ctxt, request.Name), err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
						fmt.Sprintf("Failed to create cluster: %s", request.Name),
						fmt.Sprintf("Failed to create cluster: %s Error: %v",
							request.Name,
							fmt.Errorf("Error adding subtask")),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
					}
					return
				}
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)

				// Check for provider task to complete and update the disk info
				for {
					time.Sleep(2 * time.Second)
					var providerTask models.AppTask
					if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
						util.FailTask(fmt.Sprintf("%s-Error getting sub task status while creating cluster: %s", ctxt, request.Name), err, t)
						if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
							fmt.Sprintf("Failed to create cluster: %s", request.Name),
							fmt.Sprintf("Failed to create cluster: %s Error: %v", request.Name, err),
							nil,
							nil,
							models.NOTIFICATION_ENTITY_CLUSTER,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
						}
						return
					}
					if providerTask.Completed {
						if providerTask.Status == models.TASK_STATUS_SUCCESS {
							t.UpdateStatus("Updating the monitoring configuration")
							if err := updateMonitoringPluginsForCluster(request, ctxt); err != nil {
								logger.Get().Error("%s-Updating Monitoring configuration failed on cluster:%v.Error:%v", ctxt, request.Name, err)
								t.UpdateStatus("Failed to update monitoring configuration")
							}
							t.UpdateStatus("Starting disk sync")
							if err := syncStorageDisks(request.Nodes, ctxt); err != nil {
								t.UpdateStatus("Failed to sync the Disks")
							}
							t.UpdateStatus("Initializing Monitoring Schedules")
							cluster, err = cluster_exists("name", request.Name)
							if err != nil {
								t.UpdateStatus("Failed to initialize monitoring schedules")
								logger.Get().Error("%s-Error getting cluster details for: %s. error: %v. Could not create monitoring schedules for it.", ctxt, request.Name, err)
							} else {
								ScheduleCluster(cluster.ClusterId, cluster.MonitoringInterval)
							}
							cluster, clusterFetchError := GetClusterByName(request.Name)
							if clusterFetchError != nil {
								logger.Get().Error("%s - Cluster %v not found.Err %v", ctxt, request.Name, clusterFetchError)
							}

							if err := logAuditEvent(
								EventTypes["CLUSTER_CREATED"],
								fmt.Sprintf("Created cluster: %s", request.Name),
								fmt.Sprintf("Created cluster: %s", request.Name),
								&(cluster.ClusterId),
								&(cluster.ClusterId),
								models.NOTIFICATION_ENTITY_CLUSTER,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error(
									"%s- Unable to log create cluster event. Error: %v",
									ctxt,
									err)
							}
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)

						} else { //if the task is failed????
							if err := logAuditEvent(EventTypes["CLUSTER_CREATED"],
								fmt.Sprintf("Failed to create cluster: %s", request.Name),
								fmt.Sprintf(
									"Failed to create cluster: %s Error: %v",
									request.Name,
									fmt.Errorf("Provider task failed")),
								nil,
								nil,
								models.NOTIFICATION_ENTITY_CLUSTER,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log create cluster event. Error: %v", ctxt, err)
							}
							t.UpdateStatus("Failed")
							t.Done(models.TASK_STATUS_FAILURE)
							logger.Get().Error("%s- Failed to create the cluster %s", ctxt, request.Name)
						}
						break
					}
				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(models.ENGINE_NAME, fmt.Sprintf("Create Cluster: %s", request.Name), asyncTask, nil, nil, nil); err != nil {
		logger.Get().Error("%s-Unable to create task for creating cluster: %s. error: %v", ctxt, request.Name, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for create cluster")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v for creating cluster: %s", taskId.String(), ctxt, request.Name)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) Forget_Cluster(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	// Check if cluster is already disabled, if not forget not allowed
	uuid, err := uuid.Parse(cluster_id)
	if err != nil {
		logger.Get().Error("%s-Error parsing cluster id: %s. error: %v", ctxt, cluster_id, err)
		if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
			fmt.Sprintf("Failed to forget cluster: %s", cluster_id),
			fmt.Sprintf("Failed to forget cluster: %s Error: %v", cluster_id, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing cluster id: %s", cluster_id))
		return
	}
	clusterName, err := GetClusterNameById(uuid)
	if err != nil {
		clusterName = cluster_id
	}

	ok, err := ClusterUnmanaged(*uuid)
	if err != nil {
		logger.Get().Error("%s-Error checking managed state of cluster. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
			fmt.Sprintf("Failed to forget cluster: %s", clusterName),
			fmt.Sprintf("Failed to forget cluster: %s Error: %v", clusterName, err),
			uuid,
			uuid,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
	}
	if !ok {
		logger.Get().Error("%s-Cluster: %v is not in un-managed state. Cannot run forget.", ctxt, *uuid)
		if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
			fmt.Sprintf("Failed to forget cluster: %s", clusterName),
			fmt.Sprintf("Failed to forget cluster: %s Error: %v", clusterName,
				fmt.Errorf("cluster not in unmanaged state")),
			uuid,
			uuid,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is not in un-managed state. Cannot run forget.")
		return
	}

	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for cluster forget: %v", t.ID)

				cnodes, err := getClusterNodesById(uuid)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
						fmt.Sprintf("Failed to forget cluster: %s", clusterName),
						fmt.Sprintf("Failed to forget cluster: %s Error: %v", clusterName, err),
						uuid,
						uuid,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
					}
					return
				}
				appLock, err := LockNodes(ctxt, cnodes, "Forget_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
						fmt.Sprintf("Failed to forget cluster: %s", clusterName),
						fmt.Sprintf("Failed to forget cluster: %s Error: %v", clusterName, err),
						uuid,
						uuid,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
					}
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)
				// TODO: Remove the sync jobs if any for the cluster
				// TODO: Remove the performance monitoring details for the cluster
				// TODO: Remove the collectd, salt etc configurations from the nodes

				// Ignore the cluster nodes
				if ok, err := ignoreClusterNodes(ctxt, *uuid); err != nil || !ok {
					util.FailTask(fmt.Sprintf("Error ignoring nodes for cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
						fmt.Sprintf("Failed to forget cluster: %s", clusterName),
						fmt.Sprintf("Failed to forget cluster: %s Error: %v",
							clusterName,
							fmt.Errorf("Error ignoring nodes for cluster")),
						uuid,
						uuid,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
					}
					return
				}

				// Remove storage entities for cluster
				t.UpdateStatus("Removing storage entities for cluster")
				if err := removeStorageEntities(*uuid); err != nil {
					util.FailTask(fmt.Sprintf("Error removing storage entities for cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
						fmt.Sprintf("Failed to forget cluster: %s", clusterName),
						fmt.Sprintf("Failed to forget cluster: %s Error: %v", clusterName, err),
						uuid,
						uuid,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
					}
					return
				}

				// Delete the participating nodes from DB
				t.UpdateStatus("Deleting cluster nodes")
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
				if changeInfo, err := collection.RemoveAll(bson.M{"clusterid": *uuid}); err != nil || changeInfo == nil {
					util.FailTask(fmt.Sprintf("Error deleting cluster nodes for cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
						fmt.Sprintf("Failed to forget cluster: %s", clusterName),
						fmt.Sprintf("Failed to forget cluster: %s Error: %v",
							clusterName,
							fmt.Errorf("Error deleting cluster nodes")),
						uuid,
						uuid,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
					}
					return
				}

				// Delete the cluster from DB
				t.UpdateStatus("removing the cluster")
				collection = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Remove(bson.M{"clusterid": *uuid}); err != nil {
					util.FailTask(fmt.Sprintf("Error removing the cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
						fmt.Sprintf("Failed to forget cluster: %s", clusterName),
						fmt.Sprintf("Failed to forget cluster: %s Error: %v", clusterName, err),
						uuid,
						uuid,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
					}
					return
				}
				DeleteClusterSchedule(*uuid)
				if err := logAuditEvent(EventTypes["CLUSTER_FORGOT"],
					fmt.Sprintf("Forgot cluster: %s", clusterName),
					fmt.Sprintf("Forgot cluster: %s", clusterName),
					uuid,
					uuid,
					models.NOTIFICATION_ENTITY_CLUSTER,
					&(t.ID),
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log forget cluster event. Error: %v", ctxt, err)
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(models.ENGINE_NAME, fmt.Sprintf("Forget Cluster: %s", cluster_id), asyncTask, nil, nil, nil); err != nil {
		logger.Get().Error("%s-Unable to create task to forget cluster: %v. error: %v", ctxt, *uuid, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster forget")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v to forget cluster: %v", ctxt, taskId, *uuid)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func clusterExists(clusterList []uuid.UUID, cluster uuid.UUID) bool {
	for _, c := range clusterList {
		if c == cluster {
			return true
		}
	}
	return false
}

func getSluAffectedClusters(w http.ResponseWriter, r *http.Request, ctxt string) ([]uuid.UUID, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	sluAlarmStatus := r.URL.Query()["slualarmstatus"]
	var sluAffectedClusters []uuid.UUID
	if len(sluAlarmStatus) != 0 {
		var sluFilter bson.M = make(map[string]interface{})
		var arr []interface{}
		for _, as := range sluAlarmStatus {
			if as == "" {
				continue
			}
			if s, ok := Event_severity[as]; !ok {
				logger.Get().Error("%s-Un-supported query param: %v", ctxt, sluAlarmStatus)
				HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Un-supported query param: %s", sluAlarmStatus))
				return sluAffectedClusters, fmt.Errorf("Unsupported querry parameter")
			} else {
				arr = append(arr, bson.M{"almstatus": s})
			}
		}
		if len(arr) != 0 {
			sluFilter["$or"] = arr
			var slus []models.StorageLogicalUnit
			coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
			if err := coll.Find(sluFilter).All(&slus); err != nil {
				HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the slus . error: %v", err))
				logger.Get().Error("Error getting the slus . error: %v", err)
				return sluAffectedClusters, fmt.Errorf("Unable to get slus from DB")
			}
			for _, slu := range slus {
				if !clusterExists(sluAffectedClusters, slu.ClusterId) {
					sluAffectedClusters = append(sluAffectedClusters, slu.ClusterId)
				}
			}
		}
	}
	return sluAffectedClusters, nil
}

func getClustersBySluStatus(slu_status_str string, clusters []models.Cluster) (error, []models.Cluster) {
	var sluFilter bson.M = make(map[string]interface{})
	switch slu_status_str {
	case "ok":
		sluFilter["status"] = models.SLU_STATUS_OK
	case "warning":
		sluFilter["status"] = models.SLU_STATUS_WARN
	case "error":
		sluFilter["status"] = models.SLU_STATUS_ERROR
	case "down":
		sluFilter["state"] = models.SLU_STATE_DOWN
	default:
		return fmt.Errorf("Invalid status %s for slus", slu_status_str), []models.Cluster{}
	}
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var slus []models.StorageLogicalUnit
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(sluFilter).All(&slus); err != nil {
		return fmt.Errorf("Error getting the slus. error: %v", err), []models.Cluster{}
	}

	var sluStatusFilteredClusters []models.Cluster
	filteredSlusClusterSet := util.NewSet()
	for _, slu := range slus {
		filteredSlusClusterSet.Add(slu.ClusterId)
	}
	for _, filteredClusterIdInterface := range filteredSlusClusterSet.GetElements() {
		cId := filteredClusterIdInterface.(uuid.UUID)
		for _, cluster := range clusters {
			if uuid.Equal(cId, cluster.ClusterId) {
				sluStatusFilteredClusters = append(sluStatusFilteredClusters, cluster)
			}
		}
	}
	return nil, sluStatusFilteredClusters
}

func (a *App) GET_Clusters(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	params := r.URL.Query()
	cluster_status_str := params.Get("status")
	alarmStatus := r.URL.Query()["alarmstatus"]

	var filter bson.M = make(map[string]interface{})
	if len(alarmStatus) != 0 {
		var arr []interface{}
		for _, as := range alarmStatus {
			if as == "" {
				continue
			}
			if s, ok := Event_severity[as]; !ok {
				logger.Get().Error("%s-Un-supported query param: %v", ctxt, alarmStatus)
				HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Un-supported query param: %s", alarmStatus))
				return
			} else {
				arr = append(arr, bson.M{"almstatus": s})
			}
		}
		if len(arr) != 0 {
			filter["$or"] = arr
		}
	}

	sluAffectedClusters, err := getSluAffectedClusters(w, r, ctxt)
	if err != nil {
		return
	}

	nearFull := params.Get("near_full")

	if cluster_status_str != "" {
		switch cluster_status_str {
		case "ok":
			filter["status"] = models.CLUSTER_STATUS_OK
		case "warning":
			filter["status"] = models.CLUSTER_STATUS_WARN
		case "error":
			filter["status"] = models.CLUSTER_STATUS_ERROR
		default:
			HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid status %s for cluster", cluster_status_str))
			return
		}
	}

	name := params.Get("name")

	if name != "" {
		filter["name"] = name
	}

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.Clusters
	if err := collection.Find(filter).All(&clusters); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the clusters list. error: %v", err))
		logger.Get().Error("%s-Error getting the clusters list. error: %v", ctxt, err)
		return
	}

	selectCriteria := bson.M{
		"utilizationtype":   monitoring.CLUSTER_UTILIZATION,
		"thresholdseverity": models.CRITICAL,
	}
	clusterThresholdEventsInDb, _ := fetchThresholdEvents(selectCriteria, monitoring.CLUSTER_UTILIZATION, ctxt)

	if nearFull == "true" {
		var nearFullClusters []models.Cluster
		for _, cluster := range clusters {
			for _, clusterThresholdEvent := range clusterThresholdEventsInDb {
				if uuid.Equal(cluster.ClusterId, clusterThresholdEvent.ClusterId) {
					nearFullClusters = append(nearFullClusters, cluster)
				}
			}
		}
		clusters = models.Clusters(nearFullClusters)
	}

	slu_status_str := params.Get("slu_status")

	if slu_status_str != "" {
		err, clusters = getClustersBySluStatus(slu_status_str, clusters)
		if err != nil {
			HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the clusters list. error: %v", err))
			logger.Get().Error("%s-Error getting the clusters list. error: %v", ctxt, err)
			return
		}
	}

	slu_near_full := params.Get("slu_near_full")
	if slu_near_full == "true" {
		clusters = getClusterWithNearFullSlus(ctxt, clusters)
	} else if slu_near_full != "" && slu_near_full != "false" {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Incorrect value for slu_near_full flag"))
		logger.Get().Error("%s - Incorrect value for slu_near_full flag", ctxt)
		return
	}

	if len(clusters) == 0 {
		json.NewEncoder(w).Encode([]models.Cluster{})
	} else {
		if len(r.URL.Query()["slualarmstatus"]) != 0 {
			var filteredClusters []models.Cluster
			for _, cluster := range clusters {
				if clusterExists(sluAffectedClusters, cluster.ClusterId) {
					filteredClusters = append(filteredClusters, cluster)
				}
			}
			if len(filteredClusters) != 0 {
				json.NewEncoder(w).Encode(filteredClusters)
			} else {
				json.NewEncoder(w).Encode([]models.Cluster{})
			}
		} else {
			json.NewEncoder(w).Encode(clusters)
		}
	}
}

func getClusterWithNearFullSlus(ctxt string, clusters []models.Cluster) []models.Cluster {
	selectCriteria := bson.M{
		"utilizationtype":   monitoring.SLU_UTILIZATION,
		"thresholdseverity": models.CRITICAL,
	}
	sluThresholdEventsInDb, _ := fetchThresholdEvents(selectCriteria, monitoring.SLU_UTILIZATION, ctxt)
	filteredSlusClusterSet := util.NewSet()
	for _, sluEvent := range sluThresholdEventsInDb {
		filteredSlusClusterSet.Add(sluEvent.ClusterId)
	}
	var sluStatusFilteredClusters []models.Cluster
	for _, filteredClusterIdInterface := range filteredSlusClusterSet.GetElements() {
		cId := filteredClusterIdInterface.(uuid.UUID)
		for _, cluster := range clusters {
			if uuid.Equal(cId, cluster.ClusterId) {
				sluStatusFilteredClusters = append(sluStatusFilteredClusters, cluster)
			}
		}
	}
	return sluStatusFilteredClusters
}

func (a *App) GET_Cluster(w http.ResponseWriter, r *http.Request) {
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

	cluster, err := GetCluster(cluster_id)
	if err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the cluster with id: %v. error: %v", *cluster_id, err))
		logger.Get().Error("%s-Error getting the cluster with id: %v. error: %v", ctxt, *cluster_id, err)
		return
	}
	if cluster.Name == "" {
		HttpResponse(w, http.StatusBadRequest, "Cluster not found")
		logger.Get().Error("%s-Cluster: %v not found. error: %v", ctxt, *cluster_id, err)
		return
	} else {
		json.NewEncoder(w).Encode(cluster)
	}
}

func (a *App) Unmanage_Cluster(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
			fmt.Sprintf("Failed to un-manage cluster: %s", cluster_id_str),
			fmt.Sprintf("Failed to un-manage cluster: %s Error: %v", cluster_id_str, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
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
		logger.Get().Error("%s-Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
			fmt.Sprintf("Failed to un-manage cluster: %s", clusterName),
			fmt.Sprintf("Failed to un-manage cluster: %s Error: %v", clusterName, err),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if ok {
		logger.Get().Error("%s-Cluster: %v is already in un-managed state", ctxt, *cluster_id)
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
			fmt.Sprintf("Failed to un-manage cluster: %s", clusterName),
			fmt.Sprintf(
				"Failed to un-manage cluster: %s Error: %v",
				clusterName,
				fmt.Errorf("Cluster already un-managed")),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is already in un-managed state")
		return
	}

	asyncTask := func(t *task.Task) {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for cluster unmanage: %v", t.ID)

				// TODO: Disable sync jobs for the cluster
				// TODO: Disable performance monitoring for the cluster

				t.UpdateStatus("Getting nodes of the cluster for unmanage")
				// Disable collectd, salt configurations on the nodes participating in the cluster
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
				var nodes models.Nodes
				if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
					util.FailTask(fmt.Sprintf("Error getting nodes to un-manage for cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
						fmt.Sprintf("Failed to un-manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to un-manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
					}
					return
				}

				nodes, err := getClusterNodesById(cluster_id)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
						fmt.Sprintf("Failed to un-manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to un-manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
					}
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "Unmanage_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
						fmt.Sprintf("Failed to un-manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to un-manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
					}
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)

				for _, node := range nodes {
					t.UpdateStatus("Disabling node: %s", node.Hostname)
					ok, err := GetCoreNodeManager().DisableNode(node.Hostname, ctxt)
					if err != nil || !ok {
						util.FailTask(fmt.Sprintf("Error disabling node: %s on cluster: %v", node.Hostname, *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
						if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
							fmt.Sprintf("Failed to un-manage cluster: %s", clusterName),
							fmt.Sprintf("Failed to un-manage cluster:"+
								" %s Error: %v",
								clusterName,
								fmt.Errorf("Error disabling node")),
							cluster_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_CLUSTER,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
						}
						return
					}
					//Set the node status to unmanaged
					skyringutils.Update_node_status_byId(ctxt, node.NodeId, models.NODE_STATUS_UNKNOWN)
					skyringutils.Update_node_state_byId(ctxt, node.NodeId, models.NODE_STATE_UNMANAGED)
				}

				t.UpdateStatus("Disabling post actions on the cluster")
				// Disable any POST actions on cluster
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_UNMANAGED, "status": models.CLUSTER_STATUS_UNKNOWN}}); err != nil {
					util.FailTask(fmt.Sprintf("Error disabling post actions on cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
						fmt.Sprintf("Failed to un-manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to un-manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
					}
					return
				}
				if err := logAuditEvent(EventTypes["CLUSTER_UNMANAGE"],
					fmt.Sprintf("Un-managed cluster: %s", clusterName),
					fmt.Sprintf("Un-managed cluster: %s", clusterName),
					cluster_id,
					cluster_id,
					models.NOTIFICATION_ENTITY_CLUSTER,
					&(t.ID),
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log un-manage cluster event. Error: %v", ctxt, err)
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Unmanage Cluster: %s", cluster_id_str),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create task to unmanage cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster unmanage")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v to unmanage cluster: %v", ctxt, taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) Manage_Cluster(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
			fmt.Sprintf("Failed to manage cluster: %s", cluster_id_str),
			fmt.Sprintf("Failed to manage cluster: %s Error: %v", cluster_id_str, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}
	clusterName, err := GetClusterNameById(cluster_id)
	if err != nil {
		clusterName = cluster_id_str
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s-Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
			fmt.Sprintf("Failed to manage cluster: %s", clusterName),
			fmt.Sprintf("Failed to manage cluster: %s Error: %v", clusterName, err),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if !ok {
		logger.Get().Error("%s-Cluster: %v is already in managed state", ctxt, *cluster_id)
		if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
			fmt.Sprintf("Failed to manage cluster: %s", clusterName),
			fmt.Sprintf(
				"Failed to manage cluster: %s Error: %v",
				clusterName,
				fmt.Errorf("Cluster already managed")),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is already in managed state")
		return
	}

	asyncTask := func(t *task.Task) {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for cluster manage: %v", t.ID)

				// TODO: Enable sync jobs for the cluster
				// TODO: Enable performance monitoring for the cluster

				t.UpdateStatus("Getting nodes of cluster for manage back")
				// Enable collectd, salt configurations on the nodes participating in the cluster
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
				var nodes models.Nodes
				if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
					util.FailTask(fmt.Sprintf("Error getting nodes to manage on cluster: %v", *cluster_id), fmt.Errorf("%s - %v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
						fmt.Sprintf("Failed to manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
					}
					return
				}

				nodes, err := getClusterNodesById(cluster_id)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), fmt.Errorf("%s - %v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
						fmt.Sprintf("Failed to manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
					}
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "Manage_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s - %v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
						fmt.Sprintf("Failed to manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
					}
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)

				for _, node := range nodes {
					t.UpdateStatus("Enabling node %s", node.Hostname)
					ok, err := GetCoreNodeManager().EnableNode(node.Hostname, ctxt)
					if err != nil || !ok {
						util.FailTask(fmt.Sprintf("Error enabling node: %s on cluster: %v", node.Hostname, *cluster_id), fmt.Errorf("%s - %v", ctxt, err), t)
						if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
							fmt.Sprintf("Failed to manage cluster: %s", clusterName),
							fmt.Sprintf("Failed to manage cluster: %s Error: %v",
								clusterName,
								fmt.Errorf("Error enabling node")),
							cluster_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_CLUSTER,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
						}
						return
					}
					if err := syncNodeStatus(ctxt, node); err != nil {
						logger.Get().Error("%s-Error syncing the status of the node %v: Error. %v:", ctxt, node.Hostname, err)
					}
				}

				t.UpdateStatus("Enabling post actions on the cluster")
				// Enable any POST actions on cluster
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
					util.FailTask(fmt.Sprintf("Error enabling post actions on cluster: %v", *cluster_id), fmt.Errorf("%s - %v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
						fmt.Sprintf("Failed to manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
					}
					return
				}
				if err := syncClusterStatus(ctxt, cluster_id); err != nil {
					util.FailTask(fmt.Sprintf("Error updating cluster status for the cluster: %v", *cluster_id), fmt.Errorf("%s - %v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
						fmt.Sprintf("Failed to manage cluster: %s", clusterName),
						fmt.Sprintf("Failed to manage cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
					}
					return
				}
				if err := logAuditEvent(EventTypes["CLUSTER_MANAGE"],
					fmt.Sprintf("Managed back cluster: %s", clusterName),
					fmt.Sprintf("Managed back cluster: %s", clusterName),
					cluster_id,
					cluster_id,
					models.NOTIFICATION_ENTITY_CLUSTER,
					&(t.ID),
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log manage cluster event. Error: %v", ctxt, err)
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Manage Cluster: %s", cluster_id_str),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create task to manage cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster manage")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v to manager cluster: %v", ctxt, taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) Expand_Cluster(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
			fmt.Sprintf("Failed to expand cluster: %s", cluster_id_str),
			fmt.Sprintf("Failed to expand cluster: %s Error: %v", cluster_id_str, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}
	clusterName, err := GetClusterNameById(cluster_id)
	if err != nil {
		clusterName = cluster_id_str
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s-Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
			fmt.Sprintf("Failed to expand cluster: %s", clusterName),
			fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if ok {
		logger.Get().Error("%s-Cluster: %v is in un-managed state", ctxt, *cluster_id)
		if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
			fmt.Sprintf("Failed to expand cluster: %s", clusterName),
			fmt.Sprintf(
				"Failed to expand cluster: %s Error: %v",
				clusterName,
				fmt.Errorf("Cluster is un-managed")),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	// Unmarshal the request body
	var new_nodes []models.ClusterNode
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the expand cluster request for: %v. error: %v", ctxt, *cluster_id, err)
		if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
			fmt.Sprintf("Failed to expand cluster: %s", clusterName),
			fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the expand cluster request for: %v. error: %v", *cluster_id, err))
		return
	}
	if err := json.Unmarshal(body, &new_nodes); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request expand cluster request for cluster: %v. error: %v", ctxt, *cluster_id, err)
		if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
			fmt.Sprintf("Failed to expand cluster: %s", clusterName),
			fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	// Check if provided disks already utilized
	if used, err := disks_used(new_nodes); err != nil {
		logger.Get().Error("%s-Error checking used state of disks of nodes. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
			fmt.Sprintf("Failed to expand cluster: %s", clusterName),
			fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking used state of disks of nodes")
		return
	} else if used {
		logger.Get().Error("%s-Provided disks are already used", ctxt)
		if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
			fmt.Sprintf("Failed to expand cluster: %s", clusterName),
			fmt.Sprintf(
				"Failed to expand cluster: %s Error: %v",
				clusterName,
				fmt.Sprintf("Disks already used")),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Provided disks are already used")
		return
	}

	// Expand cluster
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
				t.UpdateStatus("Started task for cluster expansion: %v", t.ID)
				nodes, err := getClusterNodesFromRequest(new_nodes)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
						fmt.Sprintf("Failed to expand cluster: %s", clusterName),
						fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
					}
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "Expand_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
						fmt.Sprintf("Failed to expand cluster: %s", clusterName),
						fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
					}
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)
				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s-Error getting provider for cluster: %v", ctxt, *cluster_id)), t)
					if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
						fmt.Sprintf("Failed to expand cluster: %s", clusterName),
						fmt.Sprintf("Failed to expand cluster: %s Error: %v",
							clusterName,
							fmt.Errorf("Error getting provider for this cluster")),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
					}
					return
				}

				t.UpdateStatus("Installing packages")
				//Install the packages
				if failedNodes := provider.ProvisionerName.Install(ctxt, t, provider.Name, new_nodes); len(failedNodes) > 0 {
					logger.Get().Error("%s-Package Installation failed for nodes:%v", ctxt, failedNodes)
					successful := removeFailedNodes(new_nodes, failedNodes)
					if len(successful) > 0 {
						body, err = json.Marshal(successful)
						if err != nil {
							util.FailTask(fmt.Sprintf("%s-", ctxt), err, t)
							if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
								fmt.Sprintf("Failed to expand cluster: %s", clusterName),
								fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
								cluster_id,
								cluster_id,
								models.NOTIFICATION_ENTITY_CLUSTER,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
							}
							return
						}
					} else {
						util.FailTask(fmt.Sprintf("%s-", ctxt), errors.New(fmt.Sprintf("Package Installation Failed for all nodes for cluster: %s", cluster_id_str)), t)
						if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
							fmt.Sprintf("Failed to expand cluster: %s", clusterName),
							fmt.Sprintf("Failed to expand cluster: %s Error: %v",
								clusterName,
								fmt.Errorf("Package installation failed")),
							cluster_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_CLUSTER,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
						}
						return
					}
				}
				t.UpdateStatus("Installing packages done. Starting Cluster Creation.")

				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, cluster_post_functions["expand_cluster"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("Error expanding cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
						fmt.Sprintf("Failed to expand cluster: %s", clusterName),
						fmt.Sprintf(
							"Failed to expand cluster: %s Error: %v",
							clusterName,
							fmt.Errorf("Provider task failed")),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
					}
					return
				}
				// Update the master task id
				providerTaskId, err = uuid.Parse(result.Data.RequestId)
				if err != nil {
					util.FailTask(fmt.Sprintf("%s-Error parsing provider task id while expand cluster: %v", ctxt, *cluster_id), err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
						fmt.Sprintf("Failed to expand cluster: %s", clusterName),
						fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
					}
					return
				}
				t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
				if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
					util.FailTask(fmt.Sprintf("%s-Error adding sub task while expand cluster: %v", ctxt, *cluster_id), err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
						fmt.Sprintf("Failed to expand cluster: %s", clusterName),
						fmt.Sprintf("Failed to expand cluster: %s Error: %v",
							clusterName,
							fmt.Errorf("Error adding subtask")),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
					}
					return
				}
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
				// Check for provider task to complete and update the disk info
				for {
					time.Sleep(2 * time.Second)

					var providerTask models.AppTask
					if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
						util.FailTask(fmt.Sprintf("%s-Error getting sub task status while expand cluster: %v", ctxt, *cluster_id), err, t)
						if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
							fmt.Sprintf("Failed to expand cluster: %s", clusterName),
							fmt.Sprintf("Failed to expand cluster: %s Error: %v", clusterName, err),
							cluster_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_CLUSTER,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
						}
						return
					}
					if providerTask.Completed {
						if providerTask.Status == models.TASK_STATUS_SUCCESS {
							t.UpdateStatus("Updating the monitoring configuration")
							if err := updateMonitoringPluginsForClusterExpand(cluster_id, nodes, ctxt); err != nil {
								logger.Get().Error("%s-Error Updating the montoring plugins for cluster:%v. Error: %v:", ctxt, *cluster_id, err)
								t.UpdateStatus("Failed to update the monitoring configuration")
							}
							t.UpdateStatus("Starting disk sync")
							if err := syncStorageDisks(new_nodes, ctxt); err != nil {
								t.UpdateStatus("Failed to sync the disks")
							}
							if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
								fmt.Sprintf("Expanded cluster: %s", clusterName),
								fmt.Sprintf("Expanded cluster: %s", clusterName),
								cluster_id,
								cluster_id,
								models.NOTIFICATION_ENTITY_CLUSTER,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
							}
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)

						} else {
							logger.Get().Error("%s-Failed to expand the cluster %s", ctxt, *cluster_id)
							if err := logAuditEvent(EventTypes["CLUSTER_EXPAND"],
								fmt.Sprintf("Failed to expand cluster: %s", clusterName),
								fmt.Sprintf(
									"Failed to expand cluster: %s Error: %v",
									clusterName,
									fmt.Errorf("Provider task failed")),
								cluster_id,
								cluster_id,
								models.NOTIFICATION_ENTITY_CLUSTER,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log expand cluster event. Error: %v", ctxt, err)
							}
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
		fmt.Sprintf("Expand Cluster: %s", cluster_id_str),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create task to expand cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster expansion")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v to expand cluster: %v", ctxt, taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) GET_ClusterNodes(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s. error: %v", cluster_id_str, err))
		return
	}
	var filter bson.M = make(map[string]interface{})
	filter["clusterid"] = cluster_id
	params := r.URL.Query()
	node_status := params.Get("status")
	if node_status != "" {
		switch node_status {
		case "ok":
			filter["status"] = models.NODE_STATUS_OK
		case "warning":
			filter["status"] = models.NODE_STATUS_WARN
		case "error":
			filter["status"] = models.NODE_STATUS_ERROR
		default:
			HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid status %s for cluster node", node_status))
			return
		}
	}
	node_role := params.Get("role")
	if node_role != "" {
		filter["roles"] = bson.M{"$in": []string{node_role}}
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var nodes models.Nodes
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(filter).All(&nodes); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the nodes for cluster: %v. error: %v", *cluster_id, err))
		logger.Get().Error("%s-Error getting the nodes for cluster: %v. error: %v", ctxt, *cluster_id, err)
		return
	}
	if len(nodes) == 0 {
		json.NewEncoder(w).Encode(models.Nodes{})
	} else {
		json.NewEncoder(w).Encode(nodes)
	}
}

func (a *App) GET_ClusterNode(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	node_id_str := vars["node-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s. error: %v", cluster_id_str, err))
		return
	}
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the node id: %s. error: %v", ctxt, node_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the node id: %s. error: %v", node_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"clusterid": *cluster_id, "nodeid": *node_id}).One(&node); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the nodes for cluster: %v. error: %v", *cluster_id, err))
		logger.Get().Error("%s-Error getting the node for cluster: %v. error: %v", ctxt, *cluster_id, err)
		return
	}
	if node.Hostname == "" {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Node: %v not found. error: %v", *node_id, err))
		logger.Get().Error("%s-Node: %v not found for cluster: %v", ctxt, *node_id, *cluster_id)
		return
	} else {
		json.NewEncoder(w).Encode(node)
	}
}

func (a *App) GET_ClusterSlus(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	alarmStatus := r.URL.Query()["alarmstatus"]

	var filter bson.M = make(map[string]interface{})
	if len(alarmStatus) != 0 {
		var arr []interface{}
		for _, as := range alarmStatus {
			if as == "" {
				continue
			}
			if s, ok := Event_severity[as]; !ok {
				logger.Get().Error("%s-Un-supported query param: %v", ctxt, alarmStatus)
				HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Un-supported query param: %s", alarmStatus))
				return
			} else {
				arr = append(arr, bson.M{"almstatus": s})
			}
		}
		if len(arr) != 0 {
			filter["$or"] = arr
		}
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	filter["clusterid"], err = uuid.Parse(cluster_id_str)

	params := r.URL.Query()

	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s, error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s, error: %v", cluster_id_str, err))
		return
	}
	if r.URL.Query().Get("storageprofile") != "" {
		filter["storageprofile"] = r.URL.Query().Get("storageprofile")
	}

	slu_status_str := params.Get("status")
	if slu_status_str != "" {
		switch slu_status_str {
		case "ok":
			filter["status"] = models.SLU_STATUS_OK
		case "warning":
			filter["status"] = models.SLU_STATUS_WARN
		case "error":
			filter["status"] = models.SLU_STATUS_ERROR
		case "down":
			filter["state"] = models.SLU_STATE_DOWN
		default:
			HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid status %s for slus", slu_status_str))
			return
		}
	}

	if r.URL.Query().Get("nodeid") != "" {
		filter["nodeid"], err = uuid.Parse(r.URL.Query().Get("nodeid"))
		if err != nil {
			logger.Get().Error("Error parsing the node id: %s, error: %v", r.URL.Query().Get("nodeid"), err)
			HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the node id: %s, error: %v", r.URL.Query().Get("nodeid"), err))
			return
		}
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var slus []models.StorageLogicalUnit
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(filter).All(&slus); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the slus for cluster: %s. error: %v", cluster_id_str, err))
		logger.Get().Error("Error getting the slus for cluster: %s. error: %v", cluster_id_str, err)
		return
	}

	if r.URL.Query().Get("near_full") == "true" {
		selectCriteria := bson.M{
			"utilizationtype":   monitoring.SLU_UTILIZATION,
			"thresholdseverity": models.CRITICAL,
			"clusterid":         filter["clusterid"],
		}
		sluThresholdEventsInDb, _ := fetchThresholdEvents(selectCriteria, monitoring.SLU_UTILIZATION, ctxt)
		var filteredSlus []models.StorageLogicalUnit
		for _, slu := range slus {
			for _, sluEvent := range sluThresholdEventsInDb {
				if uuid.Equal(slu.SluId, sluEvent.EntityId) {
					filteredSlus = append(filteredSlus, slu)
				}
			}
		}
		slus = filteredSlus
	}

	if len(slus) == 0 {
		json.NewEncoder(w).Encode([]models.StorageLogicalUnit{})
	} else {
		json.NewEncoder(w).Encode(slus)
	}
}

func (a *App) GET_ClusterSlu(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	slu_id_str := vars["slu-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s. error: %v", cluster_id_str, err))
		return
	}
	slu_id, err := uuid.Parse(slu_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the slu id: %s. error: %v", ctxt, slu_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the slu id: %s. error: %v", slu_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var slu models.StorageLogicalUnit
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(bson.M{"clusterid": *cluster_id, "sluid": *slu_id}).One(&slu); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the slu: %v for cluster: %v. error: %v", *slu_id, *cluster_id, err))
		logger.Get().Error("%s-Error getting the slu: %v for cluster: %v. error: %v", ctxt, *slu_id, *cluster_id, err)
		return
	}
	if slu.Name == "" {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Slu: %v not found.", *slu_id))
		logger.Get().Error("%s-Slu: %v not found for cluster: %v", ctxt, *slu_id, *cluster_id)
		return
	} else {
		json.NewEncoder(w).Encode(slu)
	}
}

func (a *App) PATCH_ClusterSlu(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	slu_id_str := vars["slu-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s. error: %v", ctxt, cluster_id_str, err)
		if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
			fmt.Sprintf("Failed to update slu: %s of cluster: %s", slu_id_str, cluster_id_str),
			fmt.Sprintf("Failed to update slu:%s of cluster: %s Error: %v", slu_id_str, cluster_id_str, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update slu of cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s. error: %v", cluster_id_str, err))
		return
	}
	clusterName, err := GetClusterNameById(cluster_id)
	if err != nil {
		clusterName = cluster_id_str
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
			fmt.Sprintf("Failed to update slu: %s of cluster: %s", slu_id_str, clusterName),
			fmt.Sprintf("Failed to update slu: %s of cluster: %s Error: %v", slu_id_str, clusterName, err),
			cluster_id,
			cluster_id,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update slu of cluster event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, err.Error(), ctxt)
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
				t.UpdateStatus("Started the task for updating cluster slu: %v", t.ID)

				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					logger.Get().Error("%s-Error getting provider for cluster: %v", ctxt, *cluster_id)
					if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
						fmt.Sprintf("Failed to update slu: %s of cluster: %s", slu_id_str, clusterName),
						fmt.Sprintf(
							"Failed to update slu:%s of cluster: %s Error: %v",
							slu_id_str,
							clusterName,
							fmt.Errorf("Unable to find provider")),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log update slu of cluster event. Error: %v", ctxt, err)
					}
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, cluster_post_functions["patch_slu"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s-Error updating slu: %s of cluster: %s", ctxt, cluster_id_str, cluster_id_str), err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
						fmt.Sprintf("Failed to update slu: %s of cluster: %s", cluster_id_str, clusterName),
						fmt.Sprintf(
							"Failed to update slu: %s of cluster: %s Error: %v",
							slu_id_str,
							clusterName,
							fmt.Errorf("Provider task failed")),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error(
							"%s- Unable to log create cluster event. Error: %v",
							ctxt,
							err)
					}
					return
				}
				// Update the master task id
				providerTaskId, err = uuid.Parse(result.Data.RequestId)
				if err != nil {
					util.FailTask(fmt.Sprintf("%s-Error parsing provider task id while update slu of cluster: %v", ctxt, *cluster_id), err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
						fmt.Sprintf("Failed to update slu:%s of cluster: %s", slu_id_str, clusterName),
						fmt.Sprintf("Failed to update slu: %s cluster: %s Error: %v", slu_id_str, clusterName, err),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log update slu of cluster event. Error: %v", ctxt, err)
					}
					return
				}
				t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
				if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
					util.FailTask(fmt.Sprintf("%s-Error adding sub task while update slu of cluster: %v", ctxt, *cluster_id), err, t)
					if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
						fmt.Sprintf("Failed to update slu: %s of cluster: %s", slu_id_str, clusterName),
						fmt.Sprintf("Failed to update slu: %s of cluster:"+
							" %s Error: %v",
							slu_id_str,
							clusterName,
							fmt.Errorf("Error updating subtask")),
						cluster_id,
						cluster_id,
						models.NOTIFICATION_ENTITY_CLUSTER,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log update slu of cluster event. Error: %v", ctxt, err)
					}
					return
				}
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
				// Check for provider task to complete and update the disk info
				for {
					time.Sleep(2 * time.Second)

					var providerTask models.AppTask
					if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
						util.FailTask(fmt.Sprintf("%s-Error getting sub task status while update slu of cluster: %v", ctxt, *cluster_id), err, t)
						if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
							fmt.Sprintf("Failed to update slu: %s of cluster: %s", slu_id_str, clusterName),
							fmt.Sprintf("Failed to update slu: %s of cluster: %s Error: %v", slu_id_str, clusterName, err),
							cluster_id,
							cluster_id,
							models.NOTIFICATION_ENTITY_CLUSTER,
							&(t.ID),
							ctxt); err != nil {
							logger.Get().Error("%s- Unable to log update slu of cluster event. Error: %v", ctxt, err)
						}
						return
					}
					if providerTask.Completed {
						if providerTask.Status == models.TASK_STATUS_SUCCESS {
							if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
								fmt.Sprintf("Updated slu: %s of cluster: %s", slu_id_str, clusterName),
								fmt.Sprintf("Updated slu: %s of cluster: %s", slu_id_str, clusterName),
								cluster_id,
								cluster_id,
								models.NOTIFICATION_ENTITY_CLUSTER,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log update slu of cluster event. Error: %v", ctxt, err)
							}
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)

						} else {
							logger.Get().Error("%s-Failed to update slu: %s of cluster %s", ctxt, slu_id_str, cluster_id_str)
							if err := logAuditEvent(EventTypes["CLUSTER_UPDATE_SLU"],
								fmt.Sprintf("Failed to update slu: %s of cluster: %s", slu_id_str, clusterName),
								fmt.Sprintf(
									"Failed to update slu: %s of cluster: %s Error: %v",
									slu_id_str,
									clusterName,
									fmt.Errorf("Provider task failed")),
								cluster_id,
								cluster_id,
								models.NOTIFICATION_ENTITY_CLUSTER,
								&(t.ID),
								ctxt); err != nil {
								logger.Get().Error("%s- Unable to log update slu of cluster event. Error: %v", ctxt, err)
							}
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
		fmt.Sprintf("Update slu: %s of Cluster: %s", slu_id_str, cluster_id_str),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create task to update slu: %s of cluster: %s. error: %v", ctxt, slu_id_str, cluster_id_str, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for update slu of cluster")
		return
	} else {
		logger.Get().Debug("%s-Task Created: %v to update slu: %s of cluster: %s", ctxt, taskId, slu_id_str, cluster_id_str)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) GET_ClusterConfig(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	retVal := make(map[string]interface{})
	httpStatus := http.StatusOK
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error(
			"%s-Error parsing the cluster id: %s, error: %v",
			ctxt,
			cluster_id_str,
			err)
		HttpResponse(
			w,
			http.StatusMethodNotAllowed,
			fmt.Sprintf(
				"Error parsing the cluster id: %s, error: %v",
				cluster_id_str,
				err))
		return
	}

	cluster, clusterFetchErr := GetCluster(cluster_id)
	if clusterFetchErr != nil {
		logger.Get().Error("%s - Failed to fetch cluster %v.Error %v", ctxt, *cluster_id, clusterFetchErr)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("%s - Failed to fetch cluster %v.Error %v", ctxt, *cluster_id, clusterFetchErr))
		return
	}

	var result models.RpcResponse
	provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
	if provider == nil {
		logger.Get().Error(
			"%s-Error getting the provider for cluster: %v. error: %v",
			ctxt,
			*cluster_id,
			err)
		HttpResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf(
				"Error getting the provider for cluster: %v. error: %v",
				*cluster_id,
				err))
		return
	}

	err = provider.Client.Call(
		fmt.Sprintf(
			"%s.%s",
			provider.Name,
			cluster_post_functions["config"]),
		models.RpcRequest{
			RpcRequestVars:    mux.Vars(r),
			RpcRequestData:    []byte{},
			RpcRequestContext: ctxt},
		&result)
	if err != nil || (result.Status.StatusCode != http.StatusOK) {
		logger.Get().Error(
			"%s-Error getting config details for cluster: %v. error: %v",
			ctxt,
			*cluster_id,
			err)
		httpStatus = http.StatusPartialContent
	} else {
		clusterConfigs := make(map[string]string)
		err = json.Unmarshal(result.Data.Result, &clusterConfigs)
		if err != nil {
			logger.Get().Error("%s - Failed to unmarshal cluster configurations of cluster %v. Error %v", ctxt, cluster.Name, err)
			httpStatus = http.StatusPartialContent
		}
		retVal[models.CLUSTER_CONFIGS] = clusterConfigs
	}
	retVal[models.THRESHOLD_CONFIGS] = cluster.Monitoring.Plugins

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	notifSubsColl := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_CLUSTER_NOTIFICATION_SUBSCRIPTIONS)
	var notifSub models.ClusterNotificationSubscription
	if err := notifSubsColl.Find(bson.M{"clusterid": *cluster_id}).One(&notifSub); err != nil {
		logger.Get().Error("%s - Failed to fetch notification subscriptions of cluster %v. Error %v", ctxt, cluster.Name, err)
		httpStatus = http.StatusPartialContent
	}
	retVal[models.NOTIFICATION_LIST] = notifSub
	retVal["monitoringInterval"] = cluster.MonitoringInterval

	retBytes, marshalErr := json.Marshal(retVal)
	if marshalErr != nil {
		logger.Get().Error("%s - Failed to marshal the cluster configurations of %v. Error %v", ctxt, cluster.Name, marshalErr)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("%s - Failed to marshal the cluster configurations of %v. Error %v", ctxt, cluster.Name, marshalErr))
		return
	}
	w.WriteHeader(httpStatus)
	w.Write(retBytes)
}

func disks_used(nodes []models.ClusterNode) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	for _, node := range nodes {
		uuid, err := uuid.Parse(node.NodeId)
		if err != nil {
			return false, err
		}
		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
		var storageNode models.Node
		if err := coll.Find(bson.M{"nodeid": *uuid}).One(&storageNode); err != nil {
			return false, err
		}
		for _, disk := range storageNode.StorageDisks {
			for _, device := range node.Devices {
				if disk.DevName == device.Name && disk.Used {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func removeStorageEntities(clusterId uuid.UUID) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	// Remove the storage logical units
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if changeInfo, err := coll.RemoveAll(bson.M{"clusterid": clusterId}); err != nil || changeInfo == nil {
		return err
	}

	// TODO: Remove the pools
	coll = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	if changeInfo, err := coll.RemoveAll(bson.M{"clusterid": clusterId}); err != nil || changeInfo == nil {
		return err
	}

	return nil
}

func syncStorageDisks(nodes []models.ClusterNode, ctxt string) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var fetchedNode models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	sProfiles, err := GetDbProvider().StorageProfileInterface().StorageProfiles(ctxt, nil, models.QueryOps{})
	if err != nil {
		logger.Get().Error("%s-Unable to get the storage profiles. err:%v", ctxt, err)
	}
	for _, node := range nodes {
		nodeid, err := uuid.Parse(node.NodeId)
		if err != nil {
			return err
		}
		if err := coll.Find(bson.M{"nodeid": *nodeid}).One(&fetchedNode); err != nil {
			return err
		}
		ok, err := GetCoreNodeManager().SyncStorageDisks(fetchedNode.Hostname, sProfiles, ctxt)
		if err != nil || !ok {
			return errors.New(fmt.Sprintf("%s-Error syncing storage disks for the node: %s. error: %v", ctxt, fetchedNode.Hostname, err))
		}
	}
	return nil
}

func ignoreClusterNodes(ctxt string, clusterId uuid.UUID) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := coll.Find(bson.M{"clusterid": clusterId}).All(&nodes); err != nil {
		return false, err
	}
	for _, node := range nodes {
		if ok, err := GetCoreNodeManager().IgnoreNode(node.Hostname, ctxt); err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

func updateMonitoringPluginsForCluster(request models.AddClusterRequest, ctxt string) error {
	var monitoringState models.MonitoringState
	// Check if monitoring configuration passed and update them accordingly
	if reflect.ValueOf(request.MonitoringPlugins).IsValid() && len(request.MonitoringPlugins) != 0 {
		var nodes []string
		nodesMap, nodesFetchError := util.GetNodes(request.Nodes)
		if nodesFetchError != nil {
			logger.Get().Error("%s-Error getting node details while update monitoring configuration for cluster: %s. error: %v", ctxt, request.Name, nodesFetchError)
			return nodesFetchError
		}

		for _, node := range nodesMap {
			nodes = append(nodes, node.Hostname)
		}

		if updateFailedNodesError, updateError := GetCoreNodeManager().UpdateMonitoringConfiguration(
			nodes,
			request.MonitoringPlugins,
			ctxt); len(updateFailedNodesError) != 0 {
			updateFailedNodesErrorValues, _ := util.GetMapKeys(updateFailedNodesError)
			updateFailedNodes := util.Stringify(updateFailedNodesErrorValues)
			monitoringState.StaleNodes = updateFailedNodes
			if updateError != nil {
				logger.Get().Error(updateError.Error())
			}
			logger.Get().Error("%s-Failed to update monitoring configuration on %v of cluster %v", ctxt, updateFailedNodes, request.Name)
		}

	} else {
		request.MonitoringPlugins = monitoring.GetDefaultThresholdValues()
	}
	cluster, clusterFetchError := GetClusterByName(request.Name)
	if clusterFetchError != nil {
		logger.Get().Error("%s - Cluster %v not found.Err %v", ctxt, request.Name, clusterFetchError)
		return clusterFetchError
	}
	// Udate the thresholds to db
	monitoringState.Plugins = append(cluster.Monitoring.Plugins, request.MonitoringPlugins...)
	if dbError := updatePluginsInDb(bson.M{"name": request.Name}, monitoringState); dbError != nil {
		logger.Get().Error("%s-Failed to update plugins to db: %v for cluster: %s", ctxt, dbError, request.Name)
		return dbError
	}
	return nil
}

func GetClusterByName(clusterName string) (cluster models.Cluster, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := collection.Find(bson.M{"name": clusterName}).One(&cluster); err != nil {
		return cluster, err
	}
	return cluster, nil
}

func GetClusterNameById(clusterId *uuid.UUID) (string, error) {
	var cluster models.Cluster
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := collection.Find(bson.M{"clusterid": *clusterId}).One(&cluster); err != nil {
		return "", err
	}
	return cluster.Name, nil
}

func updateMonitoringPluginsForClusterExpand(cluster_id *uuid.UUID, nodes models.Nodes, ctxt string) error {
	//Update the monitoring configuration to the new nodes
	cluster, clusterErr := GetCluster(cluster_id)
	if clusterErr != nil {
		return clusterErr
	}
	var nodeNames []string = make([]string, len(nodes))
	for index, node := range nodes {
		nodeNames[index] = node.Hostname
	}
	updateErr := forceUpdatePlugins(cluster, nodeNames, ctxt)
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func removeFailedNodes(nodes []models.ClusterNode, failed []models.ClusterNode) (diff []models.ClusterNode) {
	var found bool
	for _, node := range nodes {
		for _, fnode := range failed {
			if node.NodeId == fnode.NodeId {
				found = true
				break
			}
		}
		if !found {
			diff = append(diff, node)
		}
		found = false
	}
	return diff
}

func (a *App) GET_Slus(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error Getting the context.Err %v", err))
		return
	}

	params := r.URL.Query()
	slu_status_str := params.Get("status")

	var filter bson.M = make(map[string]interface{})
	if slu_status_str != "" {
		switch slu_status_str {
		case "ok":
			filter["status"] = models.SLU_STATUS_OK
		case "warning":
			filter["status"] = models.SLU_STATUS_WARN
		case "error":
			filter["status"] = models.SLU_STATUS_ERROR
		case "down":
			filter["state"] = models.SLU_STATE_DOWN
		default:
			HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid status %s for slus", slu_status_str))
			return
		}
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var slus []models.StorageLogicalUnit
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(filter).All(&slus); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the slus. error: %v", err))
		logger.Get().Error("%s - Error getting the slus. error: %v", ctxt, err)
		return
	}

	if params.Get("near_full") == "true" {
		selectCriteria := bson.M{
			"utilizationtype":   monitoring.SLU_UTILIZATION,
			"thresholdseverity": models.CRITICAL,
		}
		sluThresholdEventsInDb, _ := fetchThresholdEvents(selectCriteria, monitoring.SLU_UTILIZATION, ctxt)
		var filteredSlus []models.StorageLogicalUnit
		for _, slu := range slus {
			for _, sluEvent := range sluThresholdEventsInDb {
				if uuid.Equal(slu.SluId, sluEvent.EntityId) && uuid.Equal(slu.ClusterId, sluEvent.ClusterId) {
					filteredSlus = append(filteredSlus, slu)
				}
			}
		}
		slus = filteredSlus
	}

	if len(slus) == 0 {
		json.NewEncoder(w).Encode([]models.StorageLogicalUnit{})
	} else {
		json.NewEncoder(w).Encode(slus)
	}
}
