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
	"os"
)

var (
	curr_hostname, err = os.Hostname()
)

func (a *App) POST_Nodes(w http.ResponseWriter, r *http.Request) {
	var request models.AddStorageNodeRequest

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		log.Error("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}

	if request.SshPort == 0 {
		request.SshPort = models.DEFAULT_SSH_PORT
	}

	// Check if node already added
	if node, _ := node_exists("hostname", request.Hostname); node != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Node already added")
		return
	}

	// Process the request
	addAndAcceptNode(w, request)
}

func POST_AcceptUnamangedNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	var request models.UnmanagedNode

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		log.Error("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}

	// Process the request
	acceptNode(w, hostname, request.SaltFingerprint)
}

func acceptNode(w http.ResponseWriter, hostname string, fingerprint string) {
	// Validate for required fields
	if hostname == "" || fingerprint == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Required field(s) not provided")
		return
	}

	if node, err := GetCoreNodeManager().AcceptNode(hostname, fingerprint); err == nil {
		addStorageNodeToDB(w, *node)
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, "Unable to accept node")
	}
}

func addAndAcceptNode(w http.ResponseWriter, request models.AddStorageNodeRequest) {
	// Validate for required fields
	if request.Hostname == "" || request.SshFingerprint == "" || request.User == "" || request.Password == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Required field(s) not provided")
		return
	}

	// Add the node
	if node, err := GetCoreNodeManager().AddNode(
		curr_hostname,
		request.Hostname,
		uint(request.SshPort),
		request.SshFingerprint,
		request.User,
		request.Password); err == nil {
		addStorageNodeToDB(w, *node)
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, "Unable to add node")
	}
}

func addStorageNodeToDB(w http.ResponseWriter, storage_node models.StorageNode) {
	// Add the node details to the DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Insert(storage_node); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		log.Critical("Error adding the node: %v", err)
	}
	if err := json.NewEncoder(w).Encode("Added successfully"); err != nil {
		log.Error("Error: %v", err)
	}
}

func node_exists(key string, value string) (*models.StorageNode, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.StorageNode
	if err := collection.Find(bson.M{key: value}).One(&node); err != nil {
		return nil, err
	} else {
		return &node, nil
	}
}

func (a *App) GET_Nodes(w http.ResponseWriter, r *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	params := r.URL.Query()
	managed_state := params.Get("state")

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.StorageNodes
	if managed_state != "" {
		if err := collection.Find(bson.M{"managedstate": managed_state}).All(&nodes); err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			log.Error("Error getting the nodes list: %v", err)
			return
		}
	} else {
		if err := collection.Find(nil).All(&nodes); err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			log.Error("Error getting the nodes list: %v", err)
			return
		}
	}
	json.NewEncoder(w).Encode(nodes)
}

func (a *App) GET_Node(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, _ := uuid.Parse(node_id_str)

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.StorageNode
	if err := collection.Find(bson.M{"uuid": *node_id}).One(&node); err != nil {
		log.Error("Error getting the node detail: %v", err)
	}

	if node.Hostname == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Node not found")
		log.Error("Node not found: %v", err)
		return
	} else {
		json.NewEncoder(w).Encode(node)
	}
}

func GET_UnmanagedNodes(w http.ResponseWriter, r *http.Request) {
	if nodes, err := GetCoreNodeManager().GetUnmanagedNodes(); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		log.Error("Node not found: %v", err)
	} else {
		json.NewEncoder(w).Encode(nodes)
	}
}

func GetNode(node_id uuid.UUID) models.StorageNode {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.StorageNode
	if err := collection.Find(bson.M{"uuid": node_id}).One(&node); err != nil {
		log.Error("Error getting the node detail: %v", err)
	}

	return node
}
