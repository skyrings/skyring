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
	"github.com/skyrings/skyring/backend"
	"github.com/skyrings/skyring/backend/salt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/uuid"
	"github.com/skyrings/skyring/utils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
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

var (
	salt_backend = salt.New()
)

func (a *App) POST_Clusters(w http.ResponseWriter, r *http.Request) {
	var request models.AddClusterRequest

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
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
	// Get the specific provider and invoke the method
	provider := a.getProviderFromClusterType(request.Type)
	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, cluster_post_functions["create"]),
		models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: body},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error while cluster creation %v", err))
		return
	}

	// Sync the storage disk information for the cluster nodes
	if err := syncStorageDisks(request.Nodes); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	} else {
		if reflect.ValueOf(request.MonitoringPlugins).IsValid() && len(request.MonitoringPlugins) != 0 {
			var nodes []string
			nodesMap, nodesFetchError := getNodes(request.Nodes)
			logger.Get().Info("Nodes are: %v and the fetch error is %v", nodesMap, nodesFetchError)
			if nodesFetchError == nil {
				for _, node := range nodesMap {
					nodes = append(nodes, node.Hostname)
				}
				logger.Get().Info("Node names are: %v", nodes)
				salt_backend.UpdateCollectdThresholds(nodes, request.MonitoringPlugins)
			} else {
				util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func (a *App) POST_AddMonitoringPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		util.HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	var request backend.CollectdPlugin
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}
	cluster_node_names, nodesFetchError := getNodesInCluster(cluster_id)
	if nodesFetchError == nil {
		salt_backend.AddMonitoringPlugin(cluster_node_names, request)
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
	cluster, clusterFetchErr := getCluster(cluster_id)
	if clusterFetchErr != nil {
		util.HttpResponse(w, http.StatusInternalServerError, clusterFetchErr.Error())
	}
	request.Enable = true
	updatedPlugins := append(cluster.MonitoringPlugins, request)
	if dbError := updatePluginsInDb(cluster_id, updatedPlugins); dbError != nil {
		util.HttpResponse(w, http.StatusInternalServerError, dbError.Error())
	}
}

func updatePluginsInDb(cluster_id *uuid.UUID, updatedPlugins []backend.CollectdPlugin) (err error) {
	sessionCopy := db.GetDatastore().Copy()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	dbUpdateError := coll.Update(
		bson.M{"clusterid": *cluster_id},
		bson.M{"$set": bson.M{"monitoringplugins": updatedPlugins}})
	return dbUpdateError
}

func (a *App) POST_Thresholds(w http.ResponseWriter, r *http.Request) {
	var request []backend.CollectdPlugin

	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}

	if len(request) != 0 && err == nil {
		cluster_id_uuid, cluster_id_parse_error := uuid.Parse(cluster_id)
		if cluster_id_parse_error == nil {
			cluster_node_names, err := getNodesInCluster(cluster_id_uuid)
			if err == nil {
				updateErr := salt_backend.UpdateCollectdThresholds(cluster_node_names, request)
				if updateErr != nil {
					util.HttpResponse(w, http.StatusInternalServerError, updateErr.Error())
				}
				cluster, clusterFetchErr := getCluster(cluster_id_uuid)
				if clusterFetchErr != nil {
					util.HttpResponse(w, http.StatusInternalServerError, clusterFetchErr.Error())
				} else {
					updatedPlugins := backend.UpdatePlugins(cluster.MonitoringPlugins, request)
					if dbError := updatePluginsInDb(cluster_id_uuid, updatedPlugins); dbError != nil {
						util.HttpResponse(w, http.StatusInternalServerError, dbError.Error())
					}
				}
			} else {
				util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			}
		}
	}
}

func (a *App) POST_EnableMonitoringPlugins(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		util.HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	plugin_name := vars["plugin-name"]
	cluster, err := getCluster(cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	} else {
		plugin_index := -1
		for index, plugin := range cluster.MonitoringPlugins {
			if plugin.Name == plugin_name {
				if plugin.Enable == false {
					plugin_index = index
					break
				}
			}
		}
		if plugin_index == -1 {
			util.HttpResponse(w, http.StatusInternalServerError, "Plugin is either already enabled or not configured")
		}
		if backend.Contains(plugin_name, backend.SupportedCollectdPlugins) {
			nodes, nodesFetchError := getNodesInCluster(cluster_id)
			if nodesFetchError == nil {
				salt_backend.EnableCollectdPlugin(nodes, plugin_name)
				index := backend.GetPluginIndex(plugin_name, cluster.MonitoringPlugins)
				cluster.MonitoringPlugins[index].Enable = true
				if dbError := updatePluginsInDb(cluster_id, cluster.MonitoringPlugins); dbError != nil {
					util.HttpResponse(w, http.StatusInternalServerError, dbError.Error())
				}
			} else {
				util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			}
		} else {
			util.HttpResponse(w, http.StatusInternalServerError, "Unsupported plugin")
		}
	}
}

func (a *App) POST_DisableMonitoringPlugins(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		util.HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	plugin_name := vars["plugin-name"]
	cluster, err := getCluster(cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	} else {
		plugin_index := -1
		for index, plugin := range cluster.MonitoringPlugins {
			if plugin.Name == plugin_name {
				if plugin.Enable == true {
					plugin_index = index
					break
				}
			}
		}
		if plugin_index == -1 {
			util.HttpResponse(w, http.StatusInternalServerError, "Plugin is either already disabled or not configured")
		}
		nodes, nodesFetchError := getNodesInCluster(cluster_id)
		if nodesFetchError == nil {
			salt_backend.DisableCollectdPlugin(nodes, plugin_name)
			index := backend.GetPluginIndex(plugin_name, cluster.MonitoringPlugins)
			cluster.MonitoringPlugins[index].Enable = false
			if dbError := updatePluginsInDb(cluster_id, cluster.MonitoringPlugins); dbError != nil {
				util.HttpResponse(w, http.StatusInternalServerError, dbError.Error())
			}
		} else {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		}
	}
}

func getNodesInCluster(cluster_id *uuid.UUID) (cluster_node_names []string, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
		//util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the nodes for the cluster: %s", err)
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

func (a *App) REMOVE_MonitoringPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]
	uuid, err := uuid.Parse(cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Error parsing cluster id: %s", cluster_id))
		return
	}
	plugin_name := vars["plugin-name"]
	nodes, nodesFetchError := getNodesInCluster(uuid)
	if nodesFetchError == nil {
		salt_backend.RemoveCollectdPlugin(nodes, plugin_name)
		cluster, clusterFetchErr := getCluster(uuid)
		if clusterFetchErr != nil {
			util.HttpResponse(w, http.StatusInternalServerError, clusterFetchErr.Error())
			return
		}
		index := backend.GetPluginIndex(plugin_name, cluster.MonitoringPlugins)
		updatedPlugins := append(cluster.MonitoringPlugins[:index], cluster.MonitoringPlugins[index+1:]...)
		if dbError := updatePluginsInDb(uuid, updatedPlugins); dbError != nil {
			util.HttpResponse(w, http.StatusInternalServerError, dbError.Error())
		}
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
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
	ok, err := cluster_disabled(*uuid)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}
	if !ok {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is not in un-managed state. Cannot run forget.")
		return
	}

	// TODO: Remove the sync jobs if any for the cluster
	// TODO: Remove the performance monitoring details for the cluster
	// TODO: Remove the collectd, salt etc configurations from the nodes

	// Remove storage entities for cluster
	if err := removeStorageEntities(*uuid); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error removing storage for cluster: %v", err))
		return
	}

	// Delete the participating nodes from DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if changeInfo, err := collection.RemoveAll(bson.M{"clusterid": *uuid}); err != nil || changeInfo == nil {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error removing nodes: %v", err))
		return
	}

	// Delete the cluster from DB
	collection = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := collection.Remove(bson.M{"clusterid": *uuid}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error removing the cluster: %v", err)
		return
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

func (a *App) GET_MonitoringPlugins(w http.ResponseWriter, r *http.Request) {
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
		var plugins []string
		for _, plugin := range cluster.MonitoringPlugins {
			plugins = append(plugins, plugin.Name)
		}
		json.NewEncoder(w).Encode(plugins)
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

	ok, err := cluster_disabled(*cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}
	if ok {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is already in un-managed state")
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	// TODO: Disable sync jobs for the cluster
	// TODO: Disable performance monitoring for the cluster

	// Disable collectd, salt configurations on the nodes participating in the cluster
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting nodes for cluster: %v", err))
		return
	}
	for _, node := range nodes {
		ok, err := GetCoreNodeManager().DisableNode(node.Hostname)
		if err != nil || !ok {
			util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error disabling the node: %v", err))
			return
		}
	}

	// Disable any POST actions on cluster
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"enabled": false}}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
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

	ok, err := cluster_disabled(*cluster_id)
	if err != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Error checking enabled state of cluster")
		return
	}
	if !ok {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is already in managed state")
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	// TODO: Enable sync jobs for the cluster
	// TODO: Enable performance monitoring for the cluster

	// Enable collectd, salt configurations on the nodes participating in the cluster
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting nodes for cluster: %v", err))
		return
	}
	for _, node := range nodes {
		ok, err := GetCoreNodeManager().EnableNode(node.Hostname)
		if err != nil || !ok {
			util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error enabling the node: %v", err))
			return
		}
	}

	// Enable any POST actions on cluster
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"enabled": true}}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
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
	provider := a.getProviderFromClusterId(*cluster_id)
	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, cluster_post_functions["expand_cluster"]),
		models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: body},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error expanding cluster: %s", result.Status.StatusMessage))
		return
	}

	// Sync the storage disk information for the cluster nodes
	if err := syncStorageDisks(new_nodes); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
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

func cluster_disabled(cluster_id uuid.UUID) (bool, error) {
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
