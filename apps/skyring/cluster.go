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
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/utils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
)

var cluster_post_functions = map[string]string{
	"create": "CreateCluster",
	"extend": "ExtendCluster",
	"shrink": "ShrinkCluster",
}

func writeCreateResponse(w http.ResponseWriter, request models.StorageCluster, result []byte) {
	var m map[string]interface{}
	if err = json.Unmarshal(result, &m); err != nil {
		glog.Errorf("Unable to Unmarshall the result from provider : %s", err)
	}
	status := m["Status"].(float64)
	if status != http.StatusOK {
		// Add the cluster details to the DB
		request.ClusterId = uuid.NewUUID().String()
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()

		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
		if err := coll.Insert(request); err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			glog.Fatalf("Error adding the cluster: %v", err)
			return
		}

		// Update the cluster details in hosts
		for _, node := range request.Nodes {
			request.ClusterId = uuid.NewUUID().String()
			sessionCopy := db.GetDatastore().Copy()
			defer sessionCopy.Close()

			coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
			if _, err := coll.Upsert(bson.M{"hostname": node}, bson.M{"$clusterid": request.ClusterId, "$managedstate": models.NODE_STATE_USED}); err != nil {
				util.HttpResponse(w, http.StatusInternalServerError, err.Error())
				glog.Fatalf("Error updating the host details: %v", err)
				return
			}
		}
	}
	w.Write(result)
}

func writeExtendResponse(w http.ResponseWriter, request models.StorageCluster, result []byte) {
	var m map[string]interface{}
	if err = json.Unmarshal(result, &m); err != nil {
		glog.Errorf("Unable to Unmarshall the result from provider : %s", err)
	}
	status := m["Status"].(float64)
	if status != http.StatusOK {
		// Update the cluster details in hosts
		for _, node := range request.Nodes {
			request.ClusterId = uuid.NewUUID().String()
			sessionCopy := db.GetDatastore().Copy()
			defer sessionCopy.Close()

			coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
			if _, err := coll.Upsert(bson.M{"hostname": node}, bson.M{"$clusterid": request.ClusterId, "$managedstate": models.NODE_STATE_USED}); err != nil {
				util.HttpResponse(w, http.StatusInternalServerError, err.Error())
				glog.Fatalf("Error updating the host details: %v", err)
				return
			}
		}
	}
	w.Write(result)
}

func writeShrinkResponse(w http.ResponseWriter, request models.StorageCluster, result []byte) {
	var m map[string]interface{}
	if err = json.Unmarshal(result, &m); err != nil {
		glog.Errorf("Unable to Unmarshall the result from provider : %s", err)
	}
	status := m["Status"].(float64)
	if status != http.StatusOK {
		// Update the cluster details in hosts
		for _, node := range request.Nodes {
			request.ClusterId = uuid.NewUUID().String()
			sessionCopy := db.GetDatastore().Copy()
			defer sessionCopy.Close()

			coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
			if _, err := coll.Upsert(bson.M{"hostname": node}, bson.M{"$clusterid": "", "$managedstate": models.NODE_STATE_FREE}); err != nil {
				util.HttpResponse(w, http.StatusInternalServerError, err.Error())
				glog.Fatalf("Error updating the host details: %v", err)
				return
			}
		}
	}
	w.Write(result)
}

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
	if node, _ := exists("clustername", request.ClusterName); node != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Cluster already added")
		return
	}

	var result []byte
	// Get the specific provider and invoke the method
	var route conf.Route
	provider := a.getProvider(body, route)
	provider.Client.Call(fmt.Sprintf("%s.%s", request.ClusterType, cluster_post_functions["create"]), models.Args{Vars: mux.Vars(r), Request: body}, result)

	writeCreateResponse(w, request, result)
}

func (a *App) Extend_Cluster(w http.ResponseWriter, r *http.Request) {
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

	var result []byte
	// Get the specific provider and invoke the method
	var route conf.Route
	provider := a.getProvider(body, route)
	provider.Client.Call(fmt.Sprintf("%s.%s", request.ClusterType, cluster_post_functions["extend"]), models.Args{Vars: mux.Vars(r), Request: body}, result)

	writeExtendResponse(w, request, result)
}

func (a *App) Shrink_Cluster(w http.ResponseWriter, r *http.Request) {
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

	var result []byte
	// Get the specific provider and invoke the method
	var route conf.Route
	provider := a.getProvider(body, route)
	provider.Client.Call(fmt.Sprintf("%s.%s", request.ClusterType, cluster_post_functions["shrink"]), models.Args{Vars: mux.Vars(r), Request: body}, result)

	writeShrinkResponse(w, request, result)
}

func (a *App) Forget_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster
	if err := collection.Remove(bson.M{"cluster_id": cluster_id}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		glog.Errorf("Error removing the cluster: %v", err)
		return
	}

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.StorageNodes
	if err := collection.Upsert(bson.M{"clusterid": cluster_id}, bson.M{"$clusterid": "", "$managedstate": models.NODE_STATE_FREE}); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		glog.Errorf("Error updating the node details for the cluster: %v", err)
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
		glog.Errorf("Error getting the clusters list: %v", err)
		return
	}
	json.NewEncoder(w).Encode(clusters)
}

func (a *App) GET_Cluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster
	if err := collection.Find(bson.M{"cluster_id": cluster_id}).One(&cluster); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		glog.Errorf("Error getting the cluster: %v", err)
		return
	}
	json.NewEncoder(w).Encode(cluster)
}
