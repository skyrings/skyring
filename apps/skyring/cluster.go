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
	}

	storage_types = map[string]string{
		"ceph":    "block",
		"gluster": "file",
	}
)

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
				var monitoringState models.MonitoringState
				t.UpdateStatus("Started the task for cluster creation: %v", t.ID)

				nodes, err := getClusterNodesFromRequest(request.Nodes)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %s", request.Name), err, t)
					return
				}
				appLock, err := lockNodes(nodes, "POST_Clusters")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				// Get the specific provider and invoke the method
				provider := a.getProviderFromClusterType(request.Type)
				if provider == nil {
					util.FailTask(fmt.Sprintf("%s-", ctxt), errors.New(fmt.Sprintf("Error getting provider for cluster: %s", request.Name)), t)
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, cluster_post_functions["create"]),
					models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: body, RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s-Error creating cluster: %s", ctxt, request.Name), err, t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("%s-Error parsing provider task id while creating cluster: %s", ctxt, request.Name), err, t)
						return
					}
					t.UpdateStatus("Adding sub task")
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("%s-Error adding sub task while creating cluster: %s", ctxt, request.Name), err, t)
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
							util.FailTask(fmt.Sprintf("%s-Error getting sub task status while creating cluster: %s", ctxt, request.Name), err, t)
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								// Check if monitoring configuration passed and update them accordingly
								if reflect.ValueOf(request.MonitoringPlugins).IsValid() && len(request.MonitoringPlugins) != 0 {
									var nodes []string
									nodesMap, nodesFetchError := getNodes(request.Nodes)
									if nodesFetchError == nil {
										for _, node := range nodesMap {
											nodes = append(nodes, node.Hostname)
										}
										t.UpdateStatus("Updating the monitoring configuration on all nodes in the cluster")

										if updateFailedNodesError, updateError := GetCoreNodeManager().UpdateMonitoringConfiguration(nodes, request.MonitoringPlugins); len(updateFailedNodesError) != 0 {
											updateFailedNodesErrorValues, _ := util.GetMapKeys(updateFailedNodesError)
											updateFailedNodes := util.Stringify(updateFailedNodesErrorValues)
											monitoringState.StaleNodes = updateFailedNodes
											if updateError != nil {
												logger.Get().Error(updateError.Error())
											}
											logger.Get().Error("%s-Failed to update monitoring configuration on %v of cluster %v", ctxt, updateFailedNodes, request.Name)
											t.UpdateStatus(fmt.Sprintf("Failed to update the monitoring configuration on: %v for cluster: %s", updateFailedNodes, request.Name))
										}
									} else {
										t.UpdateStatus("Error getting node details")
										logger.Get().Error("%s-Error getting node details while update monitoring configuration for cluster: %s. error: %v", ctxt, request.Name, nodesFetchError)
									}
								}
								if request.MonitoringPlugins == nil || len(request.MonitoringPlugins) == 0 {
									request.MonitoringPlugins = monitoring.GetDefaultThresholdValues()
								}
								// Udate the thresholds to db
								t.UpdateStatus("Updating configuration to db")
								monitoringState.Plugins = request.MonitoringPlugins
								if dbError := updatePluginsInDb(bson.M{"name": request.Name}, monitoringState); dbError != nil {
									t.UpdateStatus("Failed with error %s", dbError.Error())
									logger.Get().Error("%s-Failed to update plugins to db: %v for cluster: %s", ctxt, dbError, request.Name)
								} else {
									t.UpdateStatus("Updated monitoring configuration to db")
								}
								t.UpdateStatus("Starting disk sync")
								if err := syncStorageDisks(request.Nodes, ctxt); err != nil {
									t.UpdateStatus("Failed")
									t.Done(models.TASK_STATUS_FAILURE)
									break
								} else {
									t.UpdateStatus("Scheduling monitoring")
									cluster, err = cluster_exists("name", request.Name)
									if err != nil {
										t.UpdateStatus("Failed with error: %v", err)
										logger.Get().Error("%s-Error getting cluster details for: %s. error: %v. Could not create monitoring schedules for it.", ctxt, request.Name, err)
										break
									}
									ScheduleCluster(cluster.ClusterId, cluster.MonitoringInterval)
									t.UpdateStatus("Success")
									t.Done(models.TASK_STATUS_SUCCESS)
									break
								}
							}
						}
					}
					return
				}
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Create Cluster: %s", request.Name), asyncTask, 600*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("%s-Unable to create task for creating cluster: %s. error: %v", ctxt, request.Name, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for create cluster")
		return
	} else {
		logger.Get().Debug("Task Created: %v for creating cluster: %s", taskId.String(), request.Name)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) POST_AddMonitoringPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", vars["cluster-id"], cluster_id_parse_error)
		HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
		return
	}
	var request monitoring.Plugin
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request. error: %v", err)
		HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Unable to unmarshal request. error: %v", err)
		HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}
	cluster, clusterFetchErr := GetCluster(cluster_id)
	if clusterFetchErr != nil {
		logger.Get().Error("Failed to add monitoring configuration for cluster: %v.Error %v", *cluster_id, clusterFetchErr)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to add monitoring configuration for cluster: %v.Error %v", *cluster_id, clusterFetchErr))
		return
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	for _, plugin := range cluster.Monitoring.Plugins {
		if plugin.Name == request.Name {
			logger.Get().Error("Plugin %v already exists on cluster %v", request.Name, cluster.Name)
			HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Plugin %v already exists in cluster %v", request.Name, cluster.Name))
			return
		}
	}
	nodes, err := getClusterNodesById(cluster_id)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("Failed to get nodes for locking for cluster: %v.Error %v", *cluster_id, err))
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to get nodes for locking for cluster: %v.Error %v", *cluster_id, err))
		return
	}
	var cluster_node_names []string
	var down_nodes []string
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
		if node.Status == models.STATUS_DOWN {
			down_nodes = append(down_nodes, node.Hostname)
		}
	}
	if len(down_nodes) == len(cluster_node_names) {
		logger.Get().Error("All nodes in cluster %v are down", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in cluster %v are down", cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var monState models.MonitoringState
				var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(down_nodes))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
				monState.StaleNodes, _ = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())

				t.UpdateStatus("Started task to add the monitoring plugin : %v", t.ID)
				appLock, err := lockNodes(nodes, "POST_AddMonitoringPlugin")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				addNodeWiseErrors, addError := GetCoreNodeManager().AddMonitoringPlugin(cluster_node_names, "", request)
				if len(addNodeWiseErrors) != 0 {
					//The only error that GetMapKeys is if it doesn't get a map as param as it takes interface for the flexibility of handling any kind of input map
					// It is guranteed that AddMonitoringPlugin specifically returns a map else it would fail much before coming here.
					nodesInErrorValues, _ := util.GetMapKeys(addNodeWiseErrors)
					nodesInError := util.Stringify(nodesInErrorValues)
					if addError != nil {
						logger.Get().Error(addError.Error())
					}
					nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(nodesInError))
					staleMonitoringNodes, _ := util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
					monState.StaleNodes = staleMonitoringNodes
					logger.Get().Error("Failed to add monitoring configuration for the cluster %v. Error :%v", cluster.Name, addNodeWiseErrors)
					t.UpdateStatus("Failed to add monitoring configuration for the cluster %v. Error :%v", cluster.Name, addNodeWiseErrors)
				}
				updatedPlugins := append(cluster.Monitoring.Plugins, request)
				t.UpdateStatus("Updating the plugins to db")
				monState.Plugins = updatedPlugins
				if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id}, monState); dbError != nil {
					util.FailTask(fmt.Sprintf("Failed to add monitoring configuration for cluster: %v", *cluster_id), dbError, t)
					return
				}
				if len(monState.StaleNodes) == len(cluster_node_names) {
					util.FailTask(fmt.Sprintf("Failed to add monitoring configuration for cluster: %v", *cluster_id), fmt.Errorf("%v", addNodeWiseErrors), t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Create Cluster: %s", request.Name), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for adding monitoring plugin for cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for add monitoring plugin")
		return
	} else {
		logger.Get().Debug("Task Created: %v for adding moniroring plugin for cluster: %v", taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func updatePluginsInDb(parameter bson.M, monitoringState models.MonitoringState) (err error) {
	sessionCopy := db.GetDatastore().Copy()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	dbUpdateError := coll.Update(parameter, bson.M{"$set": bson.M{"monitoring": monitoringState}})
	return dbUpdateError
}

func (a *App) PUT_Thresholds(w http.ResponseWriter, r *http.Request) {
	var request []monitoring.Plugin = make([]monitoring.Plugin, 0)
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the threshold update request for the cluster %v. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Unable to unmarshall the threshold update request of cluster %v. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal threshold update request on cluster %v. error: %v", cluster_id, err))
		return
	}
	cluster_id_uuid, cluster_id_parse_error := uuid.Parse(cluster_id)
	if cluster_id_parse_error != nil {
		logger.Get().Error("Failed to parse cluster id %v. error: %v", cluster_id, cluster_id_parse_error)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse cluster id %v. error: %v", cluster_id, cluster_id_parse_error))
		return
	}
	cluster, clusterFetchErr := GetCluster(cluster_id_uuid)
	if clusterFetchErr != nil {
		logger.Get().Error("Failed to get cluster with id %v. error: %v", cluster_id, clusterFetchErr)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to get cluster with id %v. error: %v", cluster_id, clusterFetchErr))
		return
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", cluster_id_uuid)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	nodes, err := getClusterNodesById(cluster_id_uuid)
	if err != nil {
		logger.Get().Error("Failed to get nodes of cluster id %v. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to get nodes of cluster id %v. error: %v", cluster_id, err))
		return
	}
	var cluster_node_names []string
	var down_nodes []string
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
		if node.Status == models.STATUS_DOWN {
			down_nodes = append(down_nodes, node.Hostname)
		}
	}
	if len(down_nodes) == len(cluster_node_names) {
		logger.Get().Error("All nodes in cluster %v are down", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in cluster %v are down", cluster.Name))
		return
	}
	if len(request) == 0 {
		logger.Get().Error("No thresholds passed for configuration in cluster %v", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("No thresholds passed for configuration in cluster %v", cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var monState models.MonitoringState
				var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(down_nodes))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
				monState.StaleNodes, _ = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
				t.UpdateStatus("Started task to update monitoring plugins configuration : %v", t.ID)
				var updatedPlugins []monitoring.Plugin
				var pluginUpdateError error
				appLock, err := lockNodes(nodes, "PUT_Thresholds")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				if updatedPlugins, pluginUpdateError = monitoring.UpdatePluginsConfigs(cluster.Monitoring.Plugins, request); pluginUpdateError != nil {
					util.FailTask(fmt.Sprintf("Failed to update thresholds for cluster: %v", *cluster_id_uuid), pluginUpdateError, t)
					return
				}
				updateConfigurationErrors, updateErr := GetCoreNodeManager().UpdateMonitoringConfiguration(cluster_node_names, request)
				if len(updateConfigurationErrors) != 0 {
					updateConfigurationErrorValues, _ := util.GetMapKeys(updateConfigurationErrors)
					updateFailedNodes := util.Stringify(updateConfigurationErrorValues)
					if updateErr != nil {
						logger.Get().Error("Failed to update thresholds for cluster: %v.Error: %v", *cluster_id_uuid, updateErr.Error())
					}
					nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(updateFailedNodes))
					staleMonitoringNodes, _ := util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
					monState.StaleNodes = staleMonitoringNodes
					logger.Get().Error("Failed to update monitoring configuration on %v of cluster %v", updateFailedNodes, cluster.Name)
					t.UpdateStatus("Failed to update monitoring configuration on %v of cluster %v", updateFailedNodes, cluster.Name)
				}
				monState.Plugins = updatedPlugins
				t.UpdateStatus("Updating new configuration to db")
				if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id_uuid}, monState); dbError != nil {
					util.FailTask(fmt.Sprintf("Failed to update thresholds for cluster: %v", *cluster_id_uuid), dbError, t)
					return
				}
				if len(monState.StaleNodes) == len(cluster_node_names) {
					util.FailTask(fmt.Sprintf("Failed to update thresholds for cluster: %v", *cluster_id_uuid), fmt.Errorf("%v", updateConfigurationErrors), t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
			}
			return
		}
	}
	if taskId, err := a.GetTaskManager().Run("Update monitoring plugins configuration", asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for update monitoring plugin configuration for cluster: %v. error: %v", *cluster_id_uuid, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for update monitoring plugin configuration")
		return
	} else {
		logger.Get().Debug("Task Created: %v for updating monitoring plugin thresholds for cluster: %v", taskId, cluster_id_uuid)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) POST_froceUpdateMonitoringConfiguration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var cluster models.Cluster
	var err error
	cluster_id, err := uuid.Parse(vars["cluster-id"])
	if err != nil {
		logger.Get().Error(err.Error())
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cluster, err = GetCluster(cluster_id); err != nil {
		logger.Get().Error(err.Error())
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	if len(cluster.Monitoring.StaleNodes) == 0 {
		logger.Get().Error("All nodes in the cluster %v have fresh monitoring configurations", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in the cluster %v have fresh monitoring configurations", cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		if pluginUpdateErr := forceUpdatePlugins(cluster, cluster.Monitoring.StaleNodes); pluginUpdateErr != nil {
			util.FailTask(fmt.Sprintf("Failed to restore monitoring configuration on cluster %v", cluster.Name), pluginUpdateErr, t)
			return
		}
		t.UpdateStatus("Success")
		t.Done(models.TASK_STATUS_SUCCESS)
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Enforce monitoring plugins update"), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for enforcing monitoring plugin update. error: %v", err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for enforcing monitoring plugin update")
		return
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func forceUpdatePlugins(cluster models.Cluster, nodes []string) error {
	var monState models.MonitoringState
	var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
	nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
	nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(nodes))
	monState.Plugins = cluster.Monitoring.Plugins
	plugin_names := make([]string, len(cluster.Monitoring.Plugins))
	for index, plugin := range cluster.Monitoring.Plugins {
		plugin_names[index] = plugin.Name
	}
	if forUpdateErrors, forceUpdatePythonError := GetCoreNodeManager().EnforceMonitoring(plugin_names, nodes, "", cluster.Monitoring.Plugins); len(forUpdateErrors) != 0 || forceUpdatePythonError != nil {
		if forceUpdatePythonError != nil {
			return fmt.Errorf("Failed to update monitoring configuration on nodes : %v of cluster: %v.Error: %v", nodes, cluster.Name, forceUpdatePythonError)
		}
		nodesInErrorValues, _ := util.GetMapKeys(forUpdateErrors)
		nodesInError := util.Stringify(nodesInErrorValues)

		nodesInSuccess := util.StringSetDiff(nodes, nodesInError)
		for _, nodeInSuccess := range util.GenerifyStringArr(nodesInSuccess) {
			nodesWithStaleMonitoringConfig.Remove(nodeInSuccess)
		}
		monState.StaleNodes, err = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())

		if dbError := updatePluginsInDb(bson.M{"clusterid": cluster.ClusterId}, monState); dbError != nil {
			if len(monState.StaleNodes) != 0 {
				return fmt.Errorf("Updating monitoring configuration failed on %v and failed to persist the failure to db.Error: %v", monState.StaleNodes, dbError)
			}
			return fmt.Errorf("Failed to update monitoring configuration to db.Error %v", dbError)
		}
		return nil
	}
	for _, nodeInSuccess := range nodes {
		nodesWithStaleMonitoringConfig.Remove(nodeInSuccess)
	}
	staleNodes, _ := util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
	monState.StaleNodes = staleNodes
	if dbError := updatePluginsInDb(bson.M{"clusterid": cluster.ClusterId}, monState); dbError != nil {
		return fmt.Errorf("Failed to update monitoring configuration to db for the cluster %v. Error %v", cluster.Name, dbError)
	}
	return nil
}

func monitoringPluginActivationDeactivations(enable bool, plugin_name string, cluster_id *uuid.UUID, w http.ResponseWriter, a *App) {
	var action string
	cluster, err := GetCluster(cluster_id)
	if err != nil {
		logger.Get().Error("Error getting cluster with id: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	plugin_index := -1
	for index, plugin := range cluster.Monitoring.Plugins {
		if plugin.Name == plugin_name {
			if plugin.Enable != enable {
				plugin_index = index
				break
			}
		}
	}
	if enable {
		action = "enable"
	} else {
		action = "disable"
	}
	if plugin_index == -1 {
		logger.Get().Error("Plugin is either already %vd or not configured on cluster: %s", action, cluster.Name)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Plugin is either already %vd or not configured", action))
		return
	}
	if !monitoring.Contains(plugin_name, monitoring.SupportedMonitoringPlugins) {
		logger.Get().Error("Unsupported plugin: %s for cluster: %s", plugin_name, cluster.Name)
		HttpResponse(w, http.StatusInternalServerError, "Unsupported plugin")
	}
	nodes, nodesFetchError := getClusterNodesById(cluster_id)
	if nodesFetchError != nil {
		logger.Get().Error("Unbale to get nodes for cluster: %v. error: %v", cluster.Name, nodesFetchError)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	var cluster_node_names []string
	var down_nodes []string
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
		if node.Status == models.STATUS_DOWN {
			down_nodes = append(down_nodes, node.Hostname)
		}
	}
	if len(down_nodes) == len(cluster_node_names) {
		logger.Get().Error("All nodes in cluster %v are down", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in cluster %v are down", cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var monState models.MonitoringState
				var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(down_nodes))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
				monState.StaleNodes, _ = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
				t.UpdateStatus("Started task to %v monitoring plugin : %v", action, t.ID)
				var actionNodeWiseFailure map[string]string
				var actionErr error
				appLock, err := lockNodes(nodes, "monitoringPluginActivationDeactivations")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				if enable {
					actionNodeWiseFailure, actionErr = GetCoreNodeManager().EnableMonitoringPlugin(cluster_node_names, plugin_name)
				} else {
					actionNodeWiseFailure, actionErr = GetCoreNodeManager().DisableMonitoringPlugin(cluster_node_names, plugin_name)
				}
				if len(actionNodeWiseFailure) != 0 {
					if actionErr != nil {
						logger.Get().Error(actionErr.Error())
					}
					//The only error that GetMapKeys is if it doesn't get a map as param as it takes interface for the flexibility of handling any kind of input map
					// It is guranteed that both EnableMonitoringPlugin and DisableMonitoringPlugin specifically returns a map else it would fail much before coming here.
					nodesInError, _ := util.GetMapKeys(actionNodeWiseFailure)
					updateFailedNodes := util.Stringify(nodesInError)
					nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(updateFailedNodes))
					staleMonitoringNodes, _ := util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
					monState.StaleNodes = staleMonitoringNodes
					logger.Get().Error("Failed to %v plugin on %v of cluster %v", action, updateFailedNodes, cluster.Name)
					t.UpdateStatus("Failed to %v plugin on %v of cluster %v", action, updateFailedNodes, cluster.Name)
				}
				index := monitoring.GetPluginIndex(plugin_name, cluster.Monitoring.Plugins)
				cluster.Monitoring.Plugins[index].Enable = enable
				t.UpdateStatus("Updating changes to db")
				monState.Plugins = cluster.Monitoring.Plugins
				if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id}, monState); dbError != nil {
					util.FailTask(fmt.Sprintf("Failed to %s plugin %s on cluster: %s", action, plugin_name, cluster.Name), dbError, t)
					return
				}
				if len(monState.StaleNodes) == len(cluster_node_names) {
					util.FailTask(fmt.Sprintf("Failed to %s plugin %s on cluster: %s", action, plugin_name, cluster.Name), fmt.Errorf("%v plugin %s failed on cluster %v", action, plugin_name, cluster.Name), t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("%s monitoring plugin: %s", action, plugin_name), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for %s monitoring plugin on cluster: %s. error: %v", action, cluster.Name, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for"+action+"monitoring plugin")
		return
	} else {
		logger.Get().Debug("Task Created: %v for %s monitoring plugin on cluster: %s", taskId, action, cluster.Name)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) POST_MonitoringPluginEnable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error("Error parsing request. error: %v", cluster_id_parse_error)
		HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	plugin_name := vars["plugin-name"]
	monitoringPluginActivationDeactivations(true, plugin_name, cluster_id, w, a)
}

func (a *App) POST_MonitoringPluginDisable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error("Error parsing the request. error: %v", cluster_id_parse_error)
		HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	plugin_name := vars["plugin-name"]
	monitoringPluginActivationDeactivations(false, plugin_name, cluster_id, w, a)
}

func (a *App) REMOVE_MonitoringPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]
	uuid, err := uuid.Parse(cluster_id)
	if err != nil {
		logger.Get().Error("Error parsing the request cluster id: %s. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, err.Error())
		return
	}
	plugin_name := vars["plugin-name"]
	cluster, clusterFetchErr := GetCluster(uuid)
	if clusterFetchErr != nil {
		logger.Get().Error("Failed to remove plugin %s for cluster: %v.Error %v", *uuid, plugin_name, clusterFetchErr)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Failed to remove plugin %s for cluster: %v.Error %v", *uuid, plugin_name, clusterFetchErr))
		return
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", *uuid)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	nodes, nodesFetchError := getClusterNodesById(uuid)
	if nodesFetchError != nil {
		logger.Get().Error("Unbale to get nodes for cluster: %v. error: %v", *uuid, nodesFetchError)
		HttpResponse(w, http.StatusInternalServerError, nodesFetchError.Error())
	}
	var cluster_node_names []string
	var down_nodes []string
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
		if node.Status == models.STATUS_DOWN {
			down_nodes = append(down_nodes, node.Hostname)
		}
	}
	if len(down_nodes) == len(cluster_node_names) {
		logger.Get().Error("All nodes in cluster %v are down", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in cluster %v are down", cluster.Name))
		return
	}
	if monitoring.GetPluginIndex(plugin_name, cluster.Monitoring.Plugins) == -1 {
		logger.Get().Error("Plugin %v already deleted on cluster %v", plugin_name, cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Plugin %v already deleted on cluster %v", plugin_name, cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var monState models.MonitoringState
				var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(down_nodes))
				monState.StaleNodes, _ = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
				t.UpdateStatus("Task created to remove monitoring plugin %v", plugin_name)
				appLock, err := lockNodes(nodes, "REMOVE_MonitoringPlugin")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				removeNodeWiseFailure, removeErr := GetCoreNodeManager().RemoveMonitoringPlugin(cluster_node_names, plugin_name)
				if len(removeNodeWiseFailure) != 0 || removeErr != nil {
					//The only error that GetMapKeys is if it doesn't get a map as param as it takes interface for the flexibility of handling any kind of input map
					// It is guranteed that RemoveMonitoringPlugin specifically returns a map else it would fail much before coming here.
					nodesInErrorValues, _ := util.GetMapKeys(removeNodeWiseFailure)
					nodesInError := util.Stringify(nodesInErrorValues)

					nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(nodesInError))
					monState.StaleNodes, err = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())

					logger.Get().Error("Failed to remove plugin %v with error %v on cluster %v", plugin_name, removeNodeWiseFailure, cluster.Name)
					if removeErr != nil {
						logger.Get().Error(removeErr.Error())
					}
					t.UpdateStatus("Failed to remove plugin %v with error %v", plugin_name, removeNodeWiseFailure)
				}
				index := monitoring.GetPluginIndex(plugin_name, cluster.Monitoring.Plugins)
				updatedPlugins := append(cluster.Monitoring.Plugins[:index], cluster.Monitoring.Plugins[index+1:]...)
				monState.Plugins = updatedPlugins
				t.UpdateStatus("Updating the plugin %s removal to db", plugin_name)
				if dbError := updatePluginsInDb(bson.M{"clusterid": uuid}, monState); dbError != nil {
					util.FailTask(fmt.Sprintf("Failed to remove plugin %s for cluster: %v", *uuid, plugin_name), dbError, t)
					return
				}
				if len(monState.StaleNodes) == len(cluster_node_names) {
					util.FailTask(fmt.Sprintf("Failed to remove plugin %s for cluster: %v", *uuid, plugin_name), fmt.Errorf("%v", removeNodeWiseFailure), t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Remove monitoring plugin : %s", plugin_name), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for remove monitoring plugin for cluster: %v. error: %v", *uuid, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for remove monitoring plugin")
		return
	} else {
		logger.Get().Debug("Task Created: %v for remove monitoring plugin for cluster: %v", taskId, *uuid)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) GET_MonitoringPlugins(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		logger.Get().Error(fmt.Sprintf("Failed to parse the cluster id with error %v", err))
		return
	}
	cluster, err := GetCluster(cluster_id)
	if err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting cluster with id: %v. error: %v", *cluster_id, err))
		logger.Get().Error(fmt.Sprintf("Failed to fetch cluster with error %v", err))
		return
	}
	if cluster.Name == "" {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Cluster with id: %v not found", *cluster_id))
		logger.Get().Error("Cluster not found")
		return
	} else {
		json.NewEncoder(w).Encode(cluster.Monitoring.Plugins)
	}
}

func GetCluster(cluster_id *uuid.UUID) (cluster models.Cluster, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).One(&cluster); err != nil {
		return cluster, err
	}
	return cluster, nil
}

func getNodesInCluster(cluster_id *uuid.UUID) (cluster_node_names []string, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
		return nil, err
	}
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
	}
	return cluster_node_names, nil
}

func getNodes(clusterNodes []models.ClusterNode) (map[uuid.UUID]models.Node, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var nodes = make(map[uuid.UUID]models.Node)
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	for _, clusterNode := range clusterNodes {
		uuid, err := uuid.Parse(clusterNode.NodeId)
		if err != nil {
			return nodes, errors.New(fmt.Sprintf("Error parsing node id: %v", clusterNode.NodeId))
		}
		var node models.Node
		if err := coll.Find(bson.M{"nodeid": *uuid}).One(&node); err != nil {
			return nodes, err
		}
		nodes[node.NodeId] = node
	}
	return nodes, nil
}

func cluster_exists(key string, value string) (*models.Cluster, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{key: value}).One(&cluster); err != nil {
		return nil, err
	} else {
		return &cluster, nil
	}
}

func (a *App) Forget_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	// Check if cluster is already disabled, if not forget not allowed
	uuid, err := uuid.Parse(cluster_id)
	if err != nil {
		logger.Get().Error("Error parsing cluster id: %s. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing cluster id: %s", cluster_id))
		return
	}
	ok, err := ClusterUnmanaged(*uuid)
	if err != nil {
		logger.Get().Error("Error checking managed state of cluster. error: %v", err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
	}
	if !ok {
		logger.Get().Error("Cluster: %v is not in un-managed state. Cannot run forget.", *uuid)
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
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *uuid), err, t)
					return
				}
				appLock, err := lockNodes(cnodes, "Forget_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				// TODO: Remove the sync jobs if any for the cluster
				// TODO: Remove the performance monitoring details for the cluster
				// TODO: Remove the collectd, salt etc configurations from the nodes

				// Ignore the cluster nodes
				if ok, err := ignoreClusterNodes(*uuid); err != nil || !ok {
					util.FailTask(fmt.Sprintf("Error ignoring nodes for cluster: %v", *uuid), err, t)
					return
				}

				// Remove storage entities for cluster
				t.UpdateStatus("Removing storage entities for cluster")
				if err := removeStorageEntities(*uuid); err != nil {
					util.FailTask(fmt.Sprintf("Error removing storage entities for cluster: %v", *uuid), err, t)
					return
				}

				// Delete the participating nodes from DB
				t.UpdateStatus("Deleting cluster nodes")
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
				if changeInfo, err := collection.RemoveAll(bson.M{"clusterid": *uuid}); err != nil || changeInfo == nil {
					util.FailTask(fmt.Sprintf("Error deleting cluster nodes for cluster: %v", *uuid), err, t)
					return
				}

				// Delete the cluster from DB
				t.UpdateStatus("removing the cluster")
				collection = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Remove(bson.M{"clusterid": *uuid}); err != nil {
					util.FailTask(fmt.Sprintf("Error removing the cluster: %v", *uuid), err, t)
					return
				}
				DeleteClusterSchedule(*uuid)
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Forget Cluster: %s", cluster_id), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task to forget cluster: %v. error: %v", *uuid, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster forget")
		return
	} else {
		logger.Get().Debug("Task Created: %v to forget cluster: %v", taskId, *uuid)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) GET_Clusters(w http.ResponseWriter, r *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.Clusters
	if err := collection.Find(nil).All(&clusters); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the clusters list. error: %v", err))
		logger.Get().Error("Error getting the clusters list. error: %v", err)
		return
	}
	if len(clusters) == 0 {
		json.NewEncoder(w).Encode([]models.Cluster{})
	} else {
		json.NewEncoder(w).Encode(clusters)
	}
}

func (a *App) GET_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	cluster, err := GetCluster(cluster_id)
	if err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the cluster with id: %v. error: %v", *cluster_id, err))
		logger.Get().Error("Error getting the cluster with id: %v. error: %v", *cluster_id, err)
		return
	}
	if cluster.Name == "" {
		HttpResponse(w, http.StatusBadRequest, "Cluster not found")
		logger.Get().Error("Cluster: %v not found. error: %v", *cluster_id, err)
		return
	} else {
		json.NewEncoder(w).Encode(cluster)
	}
}

func (a *App) Unmanage_Cluster(w http.ResponseWriter, r *http.Request) {
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
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if ok {
		logger.Get().Error("Cluster: %v is already in un-managed state", *cluster_id)
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
					util.FailTask(fmt.Sprintf("Error getting nodes to un-manage for cluster: %v", *cluster_id), err, t)
					return
				}

				nodes, err := getClusterNodesById(cluster_id)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), err, t)
					return
				}
				appLock, err := lockNodes(nodes, "Unmanage_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)

				for _, node := range nodes {
					t.UpdateStatus("Disabling node: %s", node.Hostname)
					ok, err := GetCoreNodeManager().DisableNode(node.Hostname)
					if err != nil || !ok {
						util.FailTask(fmt.Sprintf("Error disabling node: %s on cluster: %v", node.Hostname, *cluster_id), err, t)
						return
					}
				}

				t.UpdateStatus("Disabling post actions on the cluster")
				// Disable any POST actions on cluster
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_UNMANAGED}}); err != nil {
					util.FailTask(fmt.Sprintf("Error disabling post actions on cluster: %v", *cluster_id), err, t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Unmanage Cluster: %s", cluster_id_str), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task to unmanage cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster unmanage")
		return
	} else {
		logger.Get().Debug("Task Created: %v to unmanage cluster: %v", taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) Manage_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("Error checking managed state of cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if !ok {
		logger.Get().Error("Cluster: %v is already in managed state", *cluster_id)
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
					util.FailTask(fmt.Sprintf("Error getting nodes to manage on cluster: %v", *cluster_id), err, t)
					return
				}

				nodes, err := getClusterNodesById(cluster_id)
				if err != nil {
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), err, t)
					return
				}
				appLock, err := lockNodes(nodes, "Manage_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)

				for _, node := range nodes {
					t.UpdateStatus("Enabling node %s", node.Hostname)
					ok, err := GetCoreNodeManager().EnableNode(node.Hostname)
					if err != nil || !ok {
						util.FailTask(fmt.Sprintf("Error enabling node: %s on cluster: %v", node.Hostname, *cluster_id), err, t)
						return
					}
				}

				t.UpdateStatus("Enabling post actions on the cluster")
				// Enable any POST actions on cluster
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
					util.FailTask(fmt.Sprintf("Error enabling post actions on cluster: %v", *cluster_id), err, t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Manage Cluster: %s", cluster_id_str), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task to manage cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster manage")
		return
	} else {
		logger.Get().Debug("Task Created: %v to manager cluster: %v", taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) Expand_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterUnmanaged(*cluster_id)
	if err != nil {
		logger.Get().Error("Error checking managed state of cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking managed state of cluster")
		return
	}
	if ok {
		logger.Get().Error("Cluster: %v is in un-managed state", *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	// Unmarshal the request body
	var new_nodes []models.ClusterNode
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the expand cluster request for: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the expand cluster request for: %v. error: %v", *cluster_id, err))
		return
	}
	if err := json.Unmarshal(body, &new_nodes); err != nil {
		logger.Get().Error("Unable to unmarshal request expand cluster request for cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	// Check if provided disks already utilized
	if used, err := disks_used(new_nodes); err != nil {
		logger.Get().Error("Error checking used state of disks of nodes. error: %v", err)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error checking used state of disks of nodes")
		return
	} else if used {
		logger.Get().Error("Provided disks are already used")
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
					util.FailTask(fmt.Sprintf("Failed to get nodes for locking for cluster: %v", *cluster_id), err, t)
					return
				}
				appLock, err := lockNodes(nodes, "Expand_Cluster")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				provider := a.getProviderFromClusterId(*cluster_id)
				if provider == nil {
					util.FailTask("", errors.New(fmt.Sprintf("Error etting provider for cluster: %v", *cluster_id)), t)
					return
				}
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, cluster_post_functions["expand_cluster"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("Error expanding cluster: %v", *cluster_id), err, t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask(fmt.Sprintf("Error parsing provider task id while expand cluster: %v", *cluster_id), err, t)
						return
					}
					t.UpdateStatus("Adding sub task")
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask(fmt.Sprintf("Error adding sub task while expand cluster: %v", *cluster_id), err, t)
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
							util.FailTask(fmt.Sprintf("Error getting sub task status while expand cluster: %v", *cluster_id), err, t)
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								//Update the monitoring configuration to the new nodes
								cluster, clusterErr := GetCluster(cluster_id)
								if clusterErr != nil {
									logger.Get().Error("Failed to fetch cluster with id %v.Error %v", cluster.Name, clusterErr)
								} else {
									var nodeNames []string = make([]string, len(nodes))
									for index, node := range nodes {
										nodeNames[index] = node.Hostname
									}
									updateErr := forceUpdatePlugins(cluster, nodeNames)
									if updateErr != nil {
										t.UpdateStatus(updateErr.Error())
									}
								}
								t.UpdateStatus("Starting disk sync")
								if err := syncStorageDisks(new_nodes, ""); err != nil {
									t.UpdateStatus("Failed")
									t.Done(models.TASK_STATUS_FAILURE)
								} else {
									t.UpdateStatus("Success")
									t.Done(models.TASK_STATUS_SUCCESS)
								}
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
	}

	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Expand Cluster: %s", cluster_id_str), asyncTask, 600*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task to expand cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster expansion")
		return
	} else {
		logger.Get().Debug("Task Created: %v to expand cluster: %v", taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) GET_ClusterNodes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s. error: %v", cluster_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var nodes models.Nodes
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the nodes for cluster: %v. error: %v", *cluster_id, err))
		logger.Get().Error("Error getting the nodes for cluster: %v. error: %v", *cluster_id, err)
		return
	}
	if len(nodes) == 0 {
		json.NewEncoder(w).Encode(models.Nodes{})
	} else {
		json.NewEncoder(w).Encode(nodes)
	}
}

func (a *App) GET_ClusterNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	node_id_str := vars["node-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s. error: %v", cluster_id_str, err))
		return
	}
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the node id: %s. error: %v", node_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the node id: %s. error: %v", node_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"clusterid": *cluster_id, "nodeid": *node_id}).One(&node); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the nodes for cluster: %v. error: %v", *cluster_id, err))
		logger.Get().Error("Error getting the node for cluster: %v. error: %v", *cluster_id, err)
		return
	}
	if node.Hostname == "" {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Node: %v not found. error: %v", *node_id, err))
		logger.Get().Error("Node: %v not found for cluster: %v", *node_id, *cluster_id)
		return
	} else {
		json.NewEncoder(w).Encode(node)
	}
}

func (a *App) GET_ClusterSlus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s, error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s, error: %v", cluster_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var slus []models.StorageLogicalUnit
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&slus); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the slus for cluster: %v. error: %v", *cluster_id, err))
		logger.Get().Error("Error getting the slus for cluster: %v. error: %v", *cluster_id, err)
		return
	}
	if len(slus) == 0 {
		json.NewEncoder(w).Encode([]models.StorageLogicalUnit{})
	} else {
		json.NewEncoder(w).Encode(slus)
	}
}

func (a *App) GET_ClusterSlu(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	slu_id_str := vars["slu-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", cluster_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the cluster id: %s. error: %v", cluster_id_str, err))
		return
	}
	slu_id, err := uuid.Parse(slu_id_str)
	if err != nil {
		logger.Get().Error("Error parsing the slu id: %s. error: %v", slu_id_str, err)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing the slu id: %s. error: %v", slu_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var slu models.StorageLogicalUnit
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(bson.M{"clusterid": *cluster_id, "sluid": *slu_id}).One(&slu); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting the slu: %v for cluster: %v. error: %v", *slu_id, *cluster_id, err))
		logger.Get().Error("Error getting the slu: %v for cluster: %v. error: %v", *slu_id, *cluster_id, err)
		return
	}
	if slu.Name == "" {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Slu: %v not found.", *slu_id))
		logger.Get().Error("Slu: %v not found for cluster: %v", *slu_id, *cluster_id)
		return
	} else {
		json.NewEncoder(w).Encode(slu)
	}
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

func ClusterUnmanaged(cluster_id uuid.UUID) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{"clusterid": cluster_id}).One(&cluster); err != nil {
		return false, err
	}
	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		return true, nil
	} else {
		return false, nil
	}
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
	sProfiles, err := GetDbProvider().StorageProfileInterface().StorageProfiles(nil, models.QueryOps{})
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
			return errors.New(fmt.Sprintf("Error syncing storage disks for the node: %s. error: %v", fetchedNode.Hostname, err))
		}
	}
	return nil
}

func ignoreClusterNodes(clusterId uuid.UUID) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := coll.Find(bson.M{"clusterid": clusterId}).All(&nodes); err != nil {
		return false, err
	}
	for _, node := range nodes {
		if ok, err := GetCoreNodeManager().IgnoreNode(node.Hostname); err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}
