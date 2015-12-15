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
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/task"
	"github.com/skyrings/skyring/tools/uuid"
	"github.com/skyrings/skyring/utils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
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

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Check if cluster already added
	// Node need for error check as cluster would be nil in case of error
	cluster, _ := cluster_exists("name", request.Name)
	if cluster != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Cluster: %s already exists", request.Name))
		return
	}

	// Check if provided disks already utilized
	if used, err := disks_used(request.Nodes); err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking used state of disks for nodes")
		return
	} else if used {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Provided disks are already used")
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
				// Get the specific provider and invoke the method
				provider := a.getProviderFromClusterType(request.Type)
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, cluster_post_functions["create"]),
					models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: body},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask("Error creating cluster", err, t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask("Error parsing provider task id", err, t)
						return
					}
					t.UpdateStatus("Adding sub task")
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask("Error adding sub task", err, t)
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
							util.FailTask("Error getting sub task status", err, t)
							return
						}
						if providerTask.Completed {
							if providerTask.Status == models.TASK_STATUS_SUCCESS {
								t.UpdateStatus("Starting disk sync")
								if err := syncStorageDisks(request.Nodes); err != nil {
									t.UpdateStatus("Failed")
									t.Done(models.TASK_STATUS_FAILURE)
								} else {
									t.UpdateStatus("Success")
									t.Done(models.TASK_STATUS_SUCCESS)
								}
							} else if providerTask.Status == models.TASK_STATUS_FAILURE {
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
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Create Cluster: %s", request.Name), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for create cluster. error: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for create cluster")
		return
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) POST_AddMonitoringPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error(cluster_id_parse_error.Error())
		util.HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	var request monitoring.Plugin
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Unable to parse the request")
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Unable to unmarshal request")
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				cluster_node_names, nodesFetchError := getNodesInCluster(cluster_id)
				t.UpdateStatus("Started task to add the monitoring plugin : %v", t.ID)
				if nodesFetchError == nil {
					if addNodeWiseErrors, addError := GetCoreNodeManager().AddMonitoringPlugin(cluster_node_names, "", request); addError != nil || len(addNodeWiseErrors) != 0 {
						util.FailTask("Failed to add monitoring configuration", fmt.Errorf("%v", addNodeWiseErrors), t)
						return
					}
				} else {
					util.FailTask("Failed to add monitoring configuration", nodesFetchError, t)
					return
				}
				cluster, clusterFetchErr := getCluster(cluster_id)
				if clusterFetchErr != nil {
					util.FailTask("Failed to add monitoring configuration", clusterFetchErr, t)
					return
				}
				request.Enable = true
				updatedPlugins := append(cluster.MonitoringPlugins, request)
				t.UpdateStatus("Updating the plugins to db")
				if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id}, updatedPlugins); dbError != nil {
					util.FailTask("Failed to add monitoring configuration", dbError, t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Create Cluster: %s", request.Name), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for add monitoring plugin. error: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for add monitoring plugin")
		return
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func updatePluginsInDb(parameter bson.M, updatedPlugins []monitoring.Plugin) (err error) {
	logger.Get().Info("In updatePluginsInDb, the parameter is %v and plugins are %v", parameter, updatedPlugins)
	sessionCopy := db.GetDatastore().Copy()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	dbUpdateError := coll.Update(parameter, bson.M{"$set": bson.M{"monitoringplugins": updatedPlugins}})
	return dbUpdateError
}

func (a *App) PUT_Thresholds(w http.ResponseWriter, r *http.Request) {
	var request []monitoring.Plugin = make([]monitoring.Plugin, 0)
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Unable to parse the request")
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("The body is %v and the parsed plugin is %v and error is %v", body, request, err)
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request"+err.Error())
		return
	}

	if len(request) != 0 && err == nil {
		cluster_id_uuid, cluster_id_parse_error := uuid.Parse(cluster_id)
		if cluster_id_parse_error == nil {
			cluster_node_names, err := getNodesInCluster(cluster_id_uuid)
			if err == nil {
				asyncTask := func(t *task.Task) {
					for {
						select {
						case <-t.StopCh:
							return
						default:
							t.UpdateStatus("Started task to update monitoring plugins configuration : %v", t.ID)
							var updatedPlugins []monitoring.Plugin
							var pluginUpdateError error
							cluster, clusterFetchErr := getCluster(cluster_id_uuid)
							if clusterFetchErr != nil {
								util.FailTask("Failed to update thresholds", clusterFetchErr, t)
								return
							}
							if updatedPlugins, pluginUpdateError = monitoring.UpdatePluginsConfigs(cluster.MonitoringPlugins, request); pluginUpdateError != nil {
								util.FailTask("Failed to update thresholds", pluginUpdateError, t)
								return
							}
							if updateFailedNodes, updateErr := GetCoreNodeManager().UpdateMonitoringConfiguration(cluster_node_names, request); updateErr != nil || len(updateFailedNodes) != 0 {
								util.FailTask("Failed to update thresholds", fmt.Errorf("%v", updateFailedNodes), t)
								return
							}
							t.UpdateStatus("Updating new configuration to db")
							if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id_uuid}, updatedPlugins); dbError != nil {
								util.FailTask("Failed to update thresholds", dbError, t)
								return
							}
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)
							return
						}
					}
				}
				if taskId, err := a.GetTaskManager().Run("Update monitoring plugins configuration", asyncTask, 120*time.Second, nil, nil, nil); err != nil {
					logger.Get().Error("Unable to create task for update monitoring plugin configuration. error: %v", err)
					util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for update monitoring plugin configuration")
					return
				} else {
					logger.Get().Debug("Task Created: ", taskId.String())
					bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
					w.WriteHeader(http.StatusAccepted)
					w.Write(bytes)
				}
			} else {
				util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func monitoringPluginActivationDeactivations(enable bool, plugin_name string, cluster_id *uuid.UUID, w http.ResponseWriter, a *App) {
	var action string
	cluster, err := getCluster(cluster_id)
	if err != nil {
		logger.Get().Error(err.Error())
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	} else {
		plugin_index := -1
		for index, plugin := range cluster.MonitoringPlugins {
			if plugin.Name == plugin_name {
				if plugin.Enable != enable {
					plugin_index = index
					break
				}
			}
		}
		if plugin_index == -1 {
			logger.Get().Error("Plugin is either already enabled or not configured")
			util.HttpResponse(w, http.StatusInternalServerError, "Plugin is either already enabled or not configured")
			return
		}
		if monitoring.Contains(plugin_name, monitoring.SupportedMonitoringPlugins) {
			nodes, nodesFetchError := getNodesInCluster(cluster_id)
			if nodesFetchError == nil {
				asyncTask := func(t *task.Task) {
					for {
						select {
						case <-t.StopCh:
							return
						default:
							t.UpdateStatus("Started task to enable monitoring plugin : %v", t.ID)
							var actionNodeWiseFailure map[string]string
							var actionErr error
							if enable {
								action = "enable"
								actionNodeWiseFailure, actionErr = GetCoreNodeManager().EnableMonitoringPlugin(nodes, plugin_name)
							} else {
								action = "disable"
								actionNodeWiseFailure, actionErr = GetCoreNodeManager().DisableMonitoringPlugin(nodes, plugin_name)
							}
							if len(actionNodeWiseFailure) != 0 || actionErr != nil {
								util.FailTask(fmt.Sprintf("Failed to %s plugin %s", action, plugin_name), fmt.Errorf("%v", actionNodeWiseFailure), t)
								return
							}
							index := monitoring.GetPluginIndex(plugin_name, cluster.MonitoringPlugins)
							cluster.MonitoringPlugins[index].Enable = enable
							logger.Get().Info("The updated cluster is : %v", cluster)
							t.UpdateStatus("Updating changes to db")
							if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id}, cluster.MonitoringPlugins); dbError != nil {
								util.FailTask(fmt.Sprintf("Failed to %s plugin %s", action, plugin_name), dbError, t)
								return
							}
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)
							return
						}
					}
				}
				if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("%s monitoring plugin: %s", action, plugin_name), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
					logger.Get().Error("Unable to create task for %s monitoring plugin. error: %v", action, err)
					util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for"+action+"monitoring plugin")
					return
				} else {
					logger.Get().Debug("Task Created: ", taskId.String())
					bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
					w.WriteHeader(http.StatusAccepted)
					w.Write(bytes)
				}
			} else {
				util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			}
		} else {
			util.HttpResponse(w, http.StatusInternalServerError, "Unsupported plugin")
		}
	}
}

func (a *App) POST_MonitoringPluginEnable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error(cluster_id_parse_error.Error())
		util.HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	plugin_name := vars["plugin-name"]
	monitoringPluginActivationDeactivations(true, plugin_name, cluster_id, w, a)
}

func (a *App) POST_MonitoringPluginDisable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error(cluster_id_parse_error.Error())
		util.HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	plugin_name := vars["plugin-name"]
	monitoringPluginActivationDeactivations(false, plugin_name, cluster_id, w, a)
}

func (a *App) REMOVE_MonitoringPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]
	uuid, err := uuid.Parse(cluster_id)
	if err != nil {
		logger.Get().Error(err.Error())
		util.HttpResponse(w, http.StatusMethodNotAllowed, err.Error())
		return
	}
	plugin_name := vars["plugin-name"]
	nodes, nodesFetchError := getNodesInCluster(uuid)
	if nodesFetchError == nil {
		asyncTask := func(t *task.Task) {
			for {
				select {
				case <-t.StopCh:
					return
				default:
					t.UpdateStatus("Task created to remove monitoring plugin %v", plugin_name)
					if removeNodeWiseFailure, removeErr := GetCoreNodeManager().RemoveMonitoringPlugin(nodes, plugin_name); len(removeNodeWiseFailure) != 0 || removeErr != nil {
						util.FailTask(fmt.Sprintf("Failed to remove plugin %s", plugin_name), fmt.Errorf("%v", removeNodeWiseFailure), t)
						return
					}
					cluster, clusterFetchErr := getCluster(uuid)
					if clusterFetchErr != nil {
						util.FailTask(fmt.Sprintf("Failed to remove plugin %s", plugin_name), clusterFetchErr, t)
						return
					}
					index := monitoring.GetPluginIndex(plugin_name, cluster.MonitoringPlugins)
					updatedPlugins := append(cluster.MonitoringPlugins[:index], cluster.MonitoringPlugins[index+1:]...)
					t.UpdateStatus("Updating the plugin %s removal to db", plugin_name)
					if dbError := updatePluginsInDb(bson.M{"clusterid": uuid}, updatedPlugins); dbError != nil {
						util.FailTask(fmt.Sprintf("Failed to remove plugin %s", plugin_name), dbError, t)
						return
					}
					t.UpdateStatus("Success")
					t.Done(models.TASK_STATUS_SUCCESS)
					return
				}
			}
		}
		if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Remove monitoring plugin : %s", plugin_name), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
			logger.Get().Error("Unable to create task for remove monitoring plugin. error: %v", err)
			util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for remove monitoring plugin")
			return
		} else {
			logger.Get().Debug("Task Created: ", taskId.String())
			bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
			w.WriteHeader(http.StatusAccepted)
			w.Write(bytes)
		}
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) GET_MonitoringPlugins(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		logger.Get().Error(fmt.Sprintf("Failed to parse the cluster id with error %v", err))
		return
	}
	cluster, err := getCluster(cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error(fmt.Sprintf("Failed to fetch cluster with error %v", err))
		return
	}
	if cluster.Name == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Cluster not found")
		logger.Get().Error("Cluster not found")
		return
	} else {
		json.NewEncoder(w).Encode(cluster.MonitoringPlugins)
	}
}

func getCluster(cluster_id *uuid.UUID) (cluster models.Cluster, err error) {
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
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing cluster id: %s", cluster_id))
		return
	}
	ok, err := ClusterDisabled(*uuid)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}
	if !ok {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is not in un-managed state. Cannot run forget.")
		return
	}

	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for cluster forget: %v", t.ID)
				// TODO: Remove the sync jobs if any for the cluster
				// TODO: Remove the performance monitoring details for the cluster
				// TODO: Remove the collectd, salt etc configurations from the nodes

				// Ignore the cluster nodes
				if ok, err := ignoreClusterNodes(*uuid); err != nil || !ok {
					util.FailTask("Error ignoring nodes", err, t)
					return
				}

				// Remove storage entities for cluster
				t.UpdateStatus("Removing storage entities for cluster")
				if err := removeStorageEntities(*uuid); err != nil {
					util.FailTask("Error removing storage entities", err, t)
					return
				}

				// Delete the participating nodes from DB
				t.UpdateStatus("Deleting cluster nodes")
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
				if changeInfo, err := collection.RemoveAll(bson.M{"clusterid": *uuid}); err != nil || changeInfo == nil {
					util.FailTask("Error deleting cluster nodes", err, t)
					return
				}

				// Delete the cluster from DB
				t.UpdateStatus("removing the cluster")
				collection = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Remove(bson.M{"clusterid": *uuid}); err != nil {
					util.FailTask("Error removing the cluster", err, t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Forget Cluster: %s", cluster_id), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for cluster forget. error: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster forget")
		return
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
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
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the clusters list: %v", err)
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
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	cluster, err := getCluster(cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the cluster: %v", err)
		return
	}
	if cluster.Name == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Cluster not found")
		logger.Get().Error("Cluster not found: %v", err)
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
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterDisabled(*cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}
	if ok {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is already in un-managed state")
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
					util.FailTask("Error getting nodes for un-manage", err, t)
					return
				}
				for _, node := range nodes {
					t.UpdateStatus("Disabling node: %s", node.Hostname)
					ok, err := GetCoreNodeManager().DisableNode(node.Hostname)
					if err != nil || !ok {
						util.FailTask("Error disabling node", err, t)
						return
					}
				}

				t.UpdateStatus("Disabling post actions on the cluster")
				// Disable any POST actions on cluster
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"enabled": false}}); err != nil {
					util.FailTask("Error disabling post actions on cluster", err, t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Unmanage Cluster: %s", cluster_id_str), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for cluster unmanage. error: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster unmanage")
		return
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
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
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Errpr parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterDisabled(*cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}
	if !ok {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is already in managed state")
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
					util.FailTask("Error getting nodes for manage", err, t)
					return
				}
				for _, node := range nodes {
					t.UpdateStatus("Enabling node")
					ok, err := GetCoreNodeManager().EnableNode(node.Hostname)
					if err != nil || !ok {
						util.FailTask("Error enabling node", err, t)
						return
					}
				}

				t.UpdateStatus("Enabling post actions on the cluster")
				// Enable any POST actions on cluster
				collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
				if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"enabled": true}}); err != nil {
					util.FailTask("Error enabling post actions on cluster", err, t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Manage Cluster: %s", cluster_id_str), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for cluster manage. error: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster manage")
		return
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
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
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Errpr parsing the cluster id: %s", cluster_id_str))
		return
	}

	ok, err := ClusterDisabled(*cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}
	if ok {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in disabled state")
		return
	}

	// Unmarshal the request body
	var new_nodes []models.ClusterNode
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &new_nodes); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Check if provided disks already utilized
	if used, err := disks_used(new_nodes); err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking used state of disks of nodes")
		return
	} else if used {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Provided disks are already used")
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
				provider := a.getProviderFromClusterId(*cluster_id)
				err = provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, cluster_post_functions["expand_cluster"]),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask("Error expanding cluster", err, t)
					return
				} else {
					// Update the master task id
					providerTaskId, err = uuid.Parse(result.Data.RequestId)
					if err != nil {
						util.FailTask("Error parsing provider task id", err, t)
						return
					}
					t.UpdateStatus("Adding sub task")
					if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
						util.FailTask("Error adding sub task", err, t)
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
							util.FailTask("Error getting sub task status", err, t)
							return
						}
						if providerTask.Completed {
							t.UpdateStatus("Starting disk sync")
							if err := syncStorageDisks(new_nodes); err != nil {
								t.UpdateStatus("Failed")
								t.Done(models.TASK_STATUS_FAILURE)
							} else {
								t.UpdateStatus("Success")
								t.Done(models.TASK_STATUS_SUCCESS)
							}
							break
						}
					}
				}
				return
			}
		}
	}

	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Expand Cluster: %s", cluster_id_str), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for cluster expansion. error: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task creation failed for cluster expansion")
		return
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
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
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Errpr parsing the cluster id: %s", cluster_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var nodes models.Nodes
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the clusters nodes: %v", err)
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
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Errpr parsing the cluster id: %s", cluster_id_str))
		return
	}
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Errpr parsing the node id: %s", node_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"clusterid": *cluster_id, "nodeid": *node_id}).One(&node); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the clusters node: %v", err)
		return
	}
	if node.Hostname == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Node not found")
		logger.Get().Error("Node not found: %v", err)
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
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Errpr parsing the cluster id: %s", cluster_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var slus []models.StorageLogicalUnit
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&slus); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the clusters slus: %v", err)
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
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Errpr parsing the cluster id: %s", cluster_id_str))
		return
	}
	slu_id, err := uuid.Parse(slu_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Errpr parsing the slu id: %s", slu_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var slu models.StorageLogicalUnit
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(bson.M{"clusterid": *cluster_id, "sluid": *slu_id}).One(&slu); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the clusters slu: %v", err)
		return
	}
	if slu.Name == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Slu not found")
		logger.Get().Error("Slu not found: %v", err)
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

func ClusterDisabled(cluster_id uuid.UUID) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{"clusterid": cluster_id}).One(&cluster); err != nil {
		return false, err
	}
	if cluster.Enabled == false {
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

func syncStorageDisks(nodes []models.ClusterNode) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var fetchedNode models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	for _, node := range nodes {
		nodeid, err := uuid.Parse(node.NodeId)
		if err != nil {
			return err
		}
		if err := coll.Find(bson.M{"nodeid": *nodeid}).One(&fetchedNode); err != nil {
			return err
		}
		ok, err := GetCoreNodeManager().SyncStorageDisks(fetchedNode.Hostname)
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
