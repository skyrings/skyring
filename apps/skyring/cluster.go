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
		t.UpdateStatus("Started the task for cluster creation: %v", t.ID)
		// Get the specific provider and invoke the method
		provider := a.getProviderFromClusterType(request.Type)
		err = provider.Client.Call(fmt.Sprintf("%s.%s",
			provider.Name, cluster_post_functions["create"]),
			models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: body},
			&result)
		if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
			t.UpdateStatus("Failed. error: %v", err)
			t.Done()
			return
		} else {
			// Update the master task id
			providerTaskId, err = uuid.Parse(result.Data.RequestId)
			if err != nil {
				t.UpdateStatus("Failed. error: %v", err)
				t.Done()
				return
			}
			sessionCopy := db.GetDatastore().Copy()
			defer sessionCopy.Close()
			coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
			t.UpdateStatus("Updating parent task id")
			if err := coll.Update(bson.M{"id": *providerTaskId}, bson.M{"$set": bson.M{"parentid": t.ID}}); err != nil {
				t.UpdateStatus("Failed. error: %v", err)
				t.Done()
				return
			}

			// Check for provider task to complete and update the disk info
			for {
				time.Sleep(2 * time.Second)
				var providerTask models.AppTask
				if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
					t.UpdateStatus("Failed. error: %v", err)
					t.Done()
					return
				}
				if providerTask.Completed {
					t.UpdateStatus("Starting disk sync")
					if err := syncStorageDisks(request.Nodes); err != nil {
						t.UpdateStatus("Failed")
					} else {
						t.UpdateStatus("Success")
					}
					t.Done()
					break
				}
			}
		}
	}

	if taskId, err := a.GetTaskManager().Run("CreateCluster", asyncTask, nil, nil, nil); err != nil {
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

func (a *App) GET_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).One(&cluster); err != nil {
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
	var providerTaskId *uuid.UUID
	asyncTask := func(t *task.Task) {
		t.UpdateStatus("Started task for cluster expansion: %v", t.ID)
		provider := a.getProviderFromClusterId(*cluster_id)
		err = provider.Client.Call(fmt.Sprintf("%s.%s",
			provider.Name, cluster_post_functions["expand_cluster"]),
			models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body},
			&result)
		if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
			t.UpdateStatus("Failed. error: %v", err)
			t.Done()
			return
		} else {
			// Update the master task id
			providerTaskId, err = uuid.Parse(result.Data.RequestId)
			if err != nil {
				t.UpdateStatus("Failed. error: %v", err)
				t.Done()
				return
			}
			sessionCopy := db.GetDatastore().Copy()
			defer sessionCopy.Close()
			coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
			t.UpdateStatus("Updating parent task id")
			if err := coll.Update(bson.M{"id": *providerTaskId}, bson.M{"$set": bson.M{"parentid": t.ID}}); err != nil {
				t.UpdateStatus("Failed. error: %v", err)
				t.Done()
				return
			}

			// Check for provider task to complete and update the disk info
			for {
				time.Sleep(2 * time.Second)
				var providerTask models.AppTask
				if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
					t.UpdateStatus("Failed. error: %v", err)
					t.Done()
					return
				}
				if providerTask.Completed {
					t.UpdateStatus("Starting disk sync")
					if err := syncStorageDisks(new_nodes); err != nil {
						t.UpdateStatus("Failed")
					} else {
						t.UpdateStatus("Success")
					}
					t.Done()
					break
				}
			}
		}
	}

	if taskId, err := a.GetTaskManager().Run("ExpandCluster", asyncTask, nil, nil, nil); err != nil {
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
