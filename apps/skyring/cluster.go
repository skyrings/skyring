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
	"github.com/gorilla/mux"
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
)

var (
	cluster_post_functions = map[string]string{
		"create": "CreateCluster",
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
		logger.Get().Error("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}
	fmt.Println(request)

	// Check if cluster already added
	if cluster, _ := cluster_exists("clustername", request.ClusterName); cluster != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster already added")
		return
	}

	var result []byte
	// Get the specific provider and invoke the method
	provider := a.getProviderFromClusterType(request.ClusterType)
	err = provider.Client.Call(fmt.Sprintf("%s.%s", provider.Name,
		cluster_post_functions["create"]),
		models.RpcRequest{RpcRequestVars: mux.Vars(r), RpcRequestData: body},
		&result)
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, "Error while cluster creation")
		return
	}

	var m models.RpcResponse
	if err = json.Unmarshal(result, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the result from provider : %s", err)
	}
	if m.Status.StatusCode == http.StatusOK {
		if err := json.NewEncoder(w).Encode("Added successfully"); err != nil {
			logger.Get().Error("Error: %v", err)
		}
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, m.Status.StatusMessage)
	}
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

func (a *App) Forget_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	// Reject the node keys and delete the nodes
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.StorageNodes
	if err := collection.Find(bson.M{"clusterid": cluster_id}).All(&nodes); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the nodes for the cluster: %v", err)
		return
	}
	for _, node := range nodes {
		if _, err := GetCoreNodeManager().RejectNode(node.Hostname); err != nil {
			logger.Get().Error("Error rejecting the key for node %s : %v", node.Hostname, err)
			continue
		} else {
			if err := collection.Remove(bson.M{"uuid": node.UUID}); err != nil {
				logger.Get().Error("Error deleting the node: %s for the cluster: %v", node.Hostname, err)
				continue
			}
		}
	}

	// TODO:: Forget the volumes/pools of the cluster (remove entries from DB only)

	// Delete the cluster from DB
	collection = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster
	if err := collection.Remove(bson.M{"cluster_id": cluster_id}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error removing the cluster: %v", err)
		return
	}

	json.NewEncoder(w).Encode(cluster)
}

func (a *App) GET_Clusters(w http.ResponseWriter, r *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.StorageClusters
	if err := collection.Find(nil).All(&clusters); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the clusters list: %v", err)
		return
	}
	json.NewEncoder(w).Encode(clusters)
}

func (a *App) GET_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, _ := uuid.Parse(cluster_id_str)
	fmt.Println(*cluster_id)

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).One(&cluster); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Error getting the cluster: %v", err)
		return
	}
	json.NewEncoder(w).Encode(cluster)
}
