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
	}

	storage_types = map[string]string{
		"ceph":    "block",
		"gluster": "file",
	}
)

func (a *App) PATCH_Clusters(w http.ResponseWriter, r *http.Request) {
	var request map[string]interface{}
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}
	var disableautoexpand bool
	if val, ok := request["disableautoexpand"]; ok {
		disableautoexpand = val.(bool)
	} else {
		logger.Get().Error("%s-Insufficient details for updating cluster", ctxt)
		HandleHttpError(w, errors.New("Insufficient details for updating cluster"))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	clid, err := uuid.Parse(cluster_id)
	if err != nil {
		logger.Get().Error("%s-Could not parse cluster uuid: %s", ctxt, cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("could not parse cluster Uuid: %s", cluster_id))
		return
	}
	if err := collection.Find(bson.M{"clusterid": *clid}).One(&cluster); err != nil {
		logger.Get().Error("%s-Cluster: %s does not exists", ctxt, cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %s does not exists", cluster_id))
		return
	}

	if err := collection.Update(bson.M{"clusterid": *clid}, bson.M{"$set": bson.M{"autoexpand": !disableautoexpand}}); err != nil {
		logger.Get().Error("%s-Failed updating cluster. Error:%v", ctxt, err)
		HandleHttpError(w, errors.New(fmt.Sprintf("Failed updating cluster. Error:%v", err)))
		return
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
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	// Check if cluster already added
	// Node need for error check as cluster would be nil in case of error
	cluster, _ := cluster_exists("name", request.Name)
	if cluster != nil {
		logger.Get().Error("%s-Cluster: %s already exists", ctxt, request.Name)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %s already exists", request.Name))
		return
	}

	// Check if provided disks already utilized
	if used, err := disks_used(request.Nodes); err != nil {
		logger.Get().Error("%s-Error checking used state of disks for nodes. error: %v", ctxt, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking used state of disks for nodes")
		return
	} else if used {
		logger.Get().Error("%s-Provided disks are already used", ctxt)
		HttpResponse(w, http.StatusMethodNotAllowed, "Provided disks are already used")
		return
	}

	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for cluster creation: %v", t.ID)

				nodes, err := getClusterNodesFromRequest(request.Nodes)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %s", request.Name), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "POST_Clusters")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}

				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)

				// Get the specific provider and invoke the method
				provider := a.getProviderFromClusterType(request.Type)
				if provider == nil {
					util.FailTask(fmt.Sprintf("%s-", ctxt), errors.New(fmt.Sprintf("Error getting provider for cluster: %s", request.Name)), t)
					return
				}
				t.UpdateStatus("Installing packages")
				//Install the packages
				if failedNodes := provider.ProvisionerName.Install(ctxt, provider.Name, request.Nodes); len(failedNodes) > 0 {
					logger.Get().Error("%s-Package Installation failed for nodes:%v", ctxt, failedNodes)
					successful := removeFailedNodes(request.Nodes, failedNodes)
					if len(successful) > 0 {
						body, err = json.Marshal(successful)
						if err != nil {
							util.FailTask(fmt.Sprintf("%s-", ctxt), err, t)
							return
						}
					} else {
						util.FailTask(fmt.Sprintf("%s-", ctxt), errors.New(fmt.Sprintf("Package Installation Failed for all nodes for cluster: %s", request.Name)), t)
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
					return
				}
				// Update the master task id
				providerTaskId, err = uuid.Parse(result.Data.RequestId)
				if err != nil {
					util.FailTask(fmt.Sprintf("%s-Error parsing provider task id while creating cluster: %s", ctxt, request.Name), err, t)
					return
				}
				t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
				if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
					util.FailTask(fmt.Sprintf("%s-Error adding sub task while creating cluster: %s", ctxt, request.Name), err, t)
					return
				}
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)

				// Check for provider task to complete and update the disk info
				for {
					time.Sleep(2 * time.Second)
					var providerTask models.AppTask
					if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
						util.FailTask(fmt.Sprintf("%s-Error getting sub task status while creating cluster: %s", ctxt, request.Name), err, t)
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
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)

						} else { //if the task is failed????
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
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing cluster id: %s", cluster_id))
		return
	}
	ok, err := ClusterUnmanaged(*uuid)
	if err != nil {
		logger.Get().Error("%s-Error checking managed state of cluster. error: %v", ctxt, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
	}
	if !ok {
		logger.Get().Error("%s-Cluster: %v is not in un-managed state. Cannot run forget.", ctxt, *uuid)
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
					return
				}
				appLock, err := LockNodes(ctxt, cnodes, "Forget_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)
				// TODO: Remove the sync jobs if any for the cluster
				// TODO: Remove the performance monitoring details for the cluster
				// TODO: Remove the collectd, salt etc configurations from the nodes

				// Ignore the cluster nodes
				if ok, err := ignoreClusterNodes(ctxt, *uuid); err != nil || !ok {
					util.FailTask(fmt.Sprintf("Error ignoring nodes for cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}

				// Remove storage entities for cluster
				t.UpdateStatus("Removing storage entities for cluster")
				if err := removeStorageEntities(*uuid); err != nil {
					util.FailTask(fmt.Sprintf("Error removing storage entities for cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}

				// Delete the participating nodes from DB
				t.UpdateStatus("Deleting cluster nodes")
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
				if changeInfo, err := collection.RemoveAll(bson.M{"clusterid": *uuid}); err != nil || changeInfo == nil {
					util.FailTask(fmt.Sprintf("Error deleting cluster nodes for cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}

				// Delete the cluster from DB
				t.UpdateStatus("removing the cluster")
				collection = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Remove(bson.M{"clusterid": *uuid}); err != nil {
					util.FailTask(fmt.Sprintf("Error removing the cluster: %v", *uuid), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				DeleteClusterSchedule(*uuid)
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

func (a *App) GET_Clusters(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.Clusters
	if err := collection.Find(nil).All(&clusters); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the clusters list. error: %v", err))
		logger.Get().Error("%s-Error getting the clusters list. error: %v", ctxt, err)
		return
	}
	if len(clusters) == 0 {
		json.NewEncoder(w).Encode([]models.Cluster{})
	} else {
		json.NewEncoder(w).Encode(clusters)
	}
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
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s-Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if ok {
		logger.Get().Error("%s-Cluster: %v is already in un-managed state", ctxt, *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is already in un-managed state")
		return
	}

	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for cluster unmanage: %v", t.ID)
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()

				// TODO: Disable sync jobs for the cluster
				// TODO: Disable performance monitoring for the cluster

				t.UpdateStatus("Getting nodes of the cluster for unmanage")
				// Disable collectd, salt configurations on the nodes participating in the cluster
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
				var nodes models.Nodes
				if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
					util.FailTask(fmt.Sprintf("Error getting nodes to un-manage for cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}

				nodes, err := getClusterNodesById(cluster_id)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "Unmanage_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)

				for _, node := range nodes {
					t.UpdateStatus("Disabling node: %s", node.Hostname)
					ok, err := GetCoreNodeManager().DisableNode(node.Hostname, ctxt)
					if err != nil || !ok {
						util.FailTask(fmt.Sprintf("Error disabling node: %s on cluster: %v", node.Hostname, *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
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
					return
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
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s-Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if !ok {
		logger.Get().Error("%s-Cluster: %v is already in managed state", ctxt, *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is already in managed state")
		return
	}

	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for cluster manage: %v", t.ID)
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()

				// TODO: Enable sync jobs for the cluster
				// TODO: Enable performance monitoring for the cluster

				t.UpdateStatus("Getting nodes of cluster for manage back")
				// Enable collectd, salt configurations on the nodes participating in the cluster
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
				var nodes models.Nodes
				if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
					util.FailTask(fmt.Sprintf("Error getting nodes to manage on cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}

				nodes, err := getClusterNodesById(cluster_id)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "Manage_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)

				for _, node := range nodes {
					t.UpdateStatus("Enabling node %s", node.Hostname)
					ok, err := GetCoreNodeManager().EnableNode(node.Hostname, ctxt)
					if err != nil || !ok {
						util.FailTask(fmt.Sprintf("Error enabling node: %s on cluster: %v", node.Hostname, *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
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
					util.FailTask(fmt.Sprintf("Error enabling post actions on cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				if err := syncClusterStatus(ctxt, cluster_id); err != nil {
					util.FailTask(fmt.Sprintf("Error updating cluster status for the cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					return
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
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("%s-Error checking managed state of cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if ok {
		logger.Get().Error("%s-Cluster: %v is in un-managed state", ctxt, *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	// Unmarshal the request body
	var new_nodes []models.ClusterNode
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the expand cluster request for: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the expand cluster request for: %v. error: %v", *cluster_id, err))
		return
	}
	if err := json.Unmarshal(body, &new_nodes); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request expand cluster request for cluster: %v. error: %v", ctxt, *cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	// Check if provided disks already utilized
	if used, err := disks_used(new_nodes); err != nil {
		logger.Get().Error("%s-Error checking used state of disks of nodes. error: %v", ctxt, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking used state of disks of nodes")
		return
	} else if used {
		logger.Get().Error("%s-Provided disks are already used", ctxt)
		HttpResponse(w, http.StatusMethodNotAllowed, "Provided disks are already used")
		return
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
				nodes, err := getClusterNodesFromRequest(new_nodes)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				appLock, err := LockNodes(ctxt, nodes, "Expand_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)
				provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("%s-Error etting provider for cluster: %v", ctxt, *cluster_id)), t)
					return
				}

				t.UpdateStatus("Installing packages")
				//Install the packages
				if failedNodes := provider.ProvisionerName.Install(ctxt, provider.Name, new_nodes); len(failedNodes) > 0 {
					logger.Get().Error("%s-Package Installation failed for nodes:%v", ctxt, failedNodes)
					successful := removeFailedNodes(new_nodes, failedNodes)
					if len(successful) > 0 {
						body, err = json.Marshal(successful)
						if err != nil {
							util.FailTask(fmt.Sprintf("%s-", ctxt), err, t)
							return
						}
					} else {
						util.FailTask(fmt.Sprintf("%s-", ctxt), errors.New(fmt.Sprintf("Package Installation Failed for all nodes for cluster: %s", cluster_id_str)), t)
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
					return
				}
				// Update the master task id
				providerTaskId, err = uuid.Parse(result.Data.RequestId)
				if err != nil {
					util.FailTask(fmt.Sprintf("%s-Error parsing provider task id while expand cluster: %v", ctxt, *cluster_id), err, t)
					return
				}
				t.UpdateStatus(fmt.Sprintf("Started provider task: %v", *providerTaskId))
				if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
					util.FailTask(fmt.Sprintf("%s-Error adding sub task while expand cluster: %v", ctxt, *cluster_id), err, t)
					return
				}
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
				// Check for provider task to complete and update the disk info
				for {
					time.Sleep(2 * time.Second)

					var providerTask models.AppTask
					if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
						util.FailTask(fmt.Sprintf("%s-Error getting sub task status while expand cluster: %v", ctxt, *cluster_id), err, t)
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
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)

						} else {
							logger.Get().Error("%s-Failed to expand the cluster %s", ctxt, *cluster_id)
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

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var nodes models.Nodes
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
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

	var filter bson.M = make(map[string]interface{})
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	filter["clusterid"], err = uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing the cluster id: %s, error: %v", ctxt, cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s, error: %v", cluster_id_str, err))
		return
	}
	if r.URL.Query().Get("storageprofile") != "" {
		filter["storageprofile"] = r.URL.Query().Get("storageprofile")
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
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s. error: %v", cluster_id_str, err))
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, err.Error(), ctxt)
		return
	}

	provider := a.GetProviderFromClusterId(ctxt, *cluster_id)
	if provider == nil {
		logger.Get().Error("%s-Error getting provider for cluster: %v", ctxt, *cluster_id)
		return
	}
	var result models.RpcResponse
	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, cluster_post_functions["patch_slu"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body},
		&result)
	if err != nil || (result.Status.StatusCode != http.StatusAccepted) {
		logger.Get().Error("%s-Error updating the slu id: %s. error: %v", ctxt, slu_id_str, err)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error updating the slu id: %s. error: %v", slu_id_str, err))
		return

	}
	taskId, err := uuid.Parse(result.Data.RequestId)
	if err != nil {
		logger.Get().Error("%s-Error parsing provider task id while updating the slu id: %s. error: %v", ctxt, slu_id_str, err)
		return
	}

	bytes, _ := json.Marshal(models.AsyncResponse{TaskId: *taskId})
	w.WriteHeader(http.StatusAccepted)
	w.Write(bytes)

}

func disks_used(nodes []models.ClusterNode) (bool, error) {
	for _, node := range nodes {
		uuid, err := uuid.Parse(node.NodeId)
		if err != nil {
			return false, err
		}
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
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
	// Udate the thresholds to db
	monitoringState.Plugins = request.MonitoringPlugins
	if dbError := updatePluginsInDb(bson.M{"name": request.Name}, monitoringState); dbError != nil {
		logger.Get().Error("%s-Failed to update plugins to db: %v for cluster: %s", ctxt, dbError, request.Name)
		return dbError
	}
	return nil
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
			if !found {
				diff = append(diff, node)
			}
			found = false
		}
	}
	return diff
}
