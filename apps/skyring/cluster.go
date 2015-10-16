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
	"fmt"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/uuid"
	"github.com/skyrings/skyring/utils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
)

var (
	cluster_post_functions = map[string]string{
		"create":         "CreateCluster",
		"remove_storage": "RemoveStorage",
	}

	storage_types = map[string]string{
		"ceph":    "block",
		"gluster": "file",
	}
)

const (
	CLUSTER_STATUS_UP   = "up"
	CLUSTER_STATUS_DOWN = "down"
)

func (a *App) POST_Clusters(w http.ResponseWriter, r *http.Request) {
	var request models.StorageCluster

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		glog.Errorf("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}

	// Check if cluster already added
	if cluster, _ := cluster_exists("clustername", request.ClusterName); cluster != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster already added")
		return
	}

	// Check if passed disks already utilized
	if used, _ := disks_used(request.Nodes); used {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Passed disks are already used")
		return
	}

	var result models.RpcResponse
	// Get the specific provider and invoke the method
	provider := a.getProviderFromClusterType(request.ClusterType)
	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, cluster_post_functions["create"]),
		models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: body},
		&result)
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error while cluster creation %v", err))
		return
	}

	if result.Status.StatusCode == http.StatusOK {
		if err := json.NewEncoder(w).Encode(result.Status.StatusMessage); err != nil {
			glog.Errorf("Error: %v", err)
		}
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, result.Status.StatusMessage)
	}
}

func (a *App) Forget_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	// Check if cluster is already disabled, if not forget not allowed
	uuid, _ := uuid.Parse(cluster_id)
	ok, cluster, _ := cluster_disabled(*uuid)
	if !ok {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is not in um-managed state. Cannot run forget.")
		return
	}

	// Delete the participating nodes from DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.StorageNodes
	if err := collection.Find(bson.M{"clusterid": cluster_id}).All(&nodes); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		glog.Errorf("Error getting the nodes for the cluster: %v", err)
		return
	}
	for _, node := range nodes {
		if err := collection.Remove(bson.M{"uuid": node.UUID}); err != nil {
			glog.Errorf("Error deleting the node: %s for the cluster: %v", node.Hostname, err)
			continue
		}
	}

	// Remove storage entities for cluster
	var result models.RpcResponse
	provider := a.getProviderFromClusterType(cluster.ClusterType)
	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, cluster_post_functions["remove_storage"]),
		models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: nil},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error removing storage for cluster %s", result.Status.StatusMessage))
		return
	}

	// Remove the sync jobs if any for the cluster
	// Remove the performance monitoring details for the cluster
	// Remove the collectd, salt etc configurations from the nodes

	// Delete the cluster from DB
	collection = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := collection.Remove(bson.M{"cluster_id": cluster_id}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		glog.Errorf("Error removing the cluster: %v", err)
		return
	}

	json.NewEncoder(w).Encode("Done")
}

func (a *App) GET_Clusters(w http.ResponseWriter, r *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.StorageClusters
	if err := collection.Find(nil).All(&clusters); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		glog.Errorf("Error getting the clusters list: %v", err)
		return
	}
	json.NewEncoder(w).Encode(clusters)
}

func (a *App) GET_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, _ := uuid.Parse(cluster_id_str)

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).One(&cluster); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		glog.Errorf("Error getting the cluster: %v", err)
		return
	}
	json.NewEncoder(w).Encode(cluster)
}

func (a *App) Disable_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, _ := uuid.Parse(cluster_id_str)

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster

	// TODO: Disable sync jobs for the cluster
	// TODO: Disable performance monitoring for the cluster

	// Disable collectd, salt configurations on the nodes participating in the cluster
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).One(&cluster); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, node := range cluster.Nodes {
		fmt.Println("Stopping services on " + node.Hostname)
		ok, err := GetCoreNodeManager().StopAndDisableService(node.Hostname, "collectd")
		if err != nil || !ok {
			util.HttpResponse(w, http.StatusInternalServerError, "Error stopping the services on node")
			return
		}
		fmt.Println("Disabling salt communication for " + node.Hostname)
		ok, err = GetCoreNodeManager().RejectNode(node.Hostname)
		if err != nil || !ok {
			util.HttpResponse(w, http.StatusInternalServerError, "Error disabling the salt communication with node")
			return
		}
		// Disable any POST actions for participating nodes
		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
		if err := coll.Update(bson.M{"hostname": node.Hostname}, bson.M{"$set": bson.M{"administrativestatus": models.UNMANAGED}}); err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error updating nodes state information %v", err))
			return
		}
	}

	// Disable any POST actions on cluster
	if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"administrativestatus": models.CLUSTER_STATUS_INACTIVE}}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode("Cluster disabled")
}

func (a *App) Enable_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, _ := uuid.Parse(cluster_id_str)

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster

	// TODO: Enable sync jobs for the cluster
	// TODO: Enable performance monitoring for the cluster

	// Enable collectd, salt configurations on the nodes participating in the cluster
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).One(&cluster); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, node := range cluster.Nodes {
		ok, err := GetCoreNodeManager().EnableAndStartService(node.Hostname, "collectd")
		if err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error starting the services on node %v", err))
			return
		}
		fmt.Println(ok)
		ok, err = GetCoreNodeManager().AcceptRejectedNode(node.Hostname)
		if err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error accepting node %v", err))
			return
		}
		fmt.Println(ok)
		// Enable any POST actions for participating nodes
		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
		if err := coll.Update(bson.M{"hostname": node.Hostname}, bson.M{"$set": bson.M{"administrativestatus": models.USED}}); err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error updating nodes state information %v", err))
			return
		}
		fmt.Println("update node status")
	}

	// Enable any POST actions on cluster
	if err := collection.Update(bson.M{"clusterid": *cluster_id}, bson.M{"$set": bson.M{"administrativestatus": models.CLUSTER_STATUS_ACTIVE_AND_AVAILABLE}}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	fmt.Println("updated cluster status")

	json.NewEncoder(w).Encode("Cluster enabled")
}

func (a *App) Expand_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, _ := uuid.Parse(cluster_id_str)

	json.NewEncoder(w).Encode(cluster_id)
}

func cluster_exists(key string, value string) (*models.StorageCluster, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster
	if err := collection.Find(bson.M{key: value}).One(&cluster); err != nil {
		return nil, err
	} else {
		return &cluster, nil
	}
}

func disks_used(nodes []models.ClusterNode) (bool, error) {
	for _, node := range nodes {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
		var storageNode models.StorageNode
		if err := coll.Find(bson.M{"hostname": node.Hostname}).One(&storageNode); err != nil {
			return false, err
		}
		for _, disk := range storageNode.StorageDisks {
			for _, diskname := range node.Disks {
				if disk.Disk.DevName == diskname && disk.AdministrativeStatus == models.USED {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func cluster_disabled(cluster_id uuid.UUID) (bool, *models.StorageCluster, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster
	if err := collection.Find(bson.M{"clusterid": cluster_id}).One(&cluster); err != nil {
		return false, nil, err
	}
	if cluster.AdministrativeStatus == models.CLUSTER_STATUS_INACTIVE {
		return true, &cluster, nil
	} else {
		return false, &cluster, nil
	}
}
