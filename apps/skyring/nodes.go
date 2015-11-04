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
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/task"
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
		logger.Get().Error("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}

	// Check if node already added
	if node, _ := node_exists("hostname", request.Hostname); node != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Node already added")
		return
	}

	if request.SshPort == 0 {
		request.SshPort = models.DEFAULT_SSH_PORT
	}

	// Validate for required fields
	if request.Hostname == "" || request.SshFingerprint == "" || request.User == "" || request.Password == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Required field(s) not provided")
		return
	}

	asyncTask := func(t *task.Task) {
		t.UpdateStatus("started the task for addAndAcceptNode: %s", t.ID)
		// Process the request
		if err := addAndAcceptNode(w, request, t); err != nil {
			t.UpdateStatus("Failed")
		} else {
			t.UpdateStatus("Success")
		}
		t.Done()
	}
	if taskId, err := task.GetManager().Run("addAndAcceptNode", asyncTask); err != nil {
		logger.Get().Error("Unable to create the task for addAndAcceptNode", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed")

	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func POST_AcceptUnamangedNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	var request models.UnmanagedNode

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

	// Validate for required fields
	if hostname == "" || request.SaltFingerprint == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Required field(s) not provided")
		return
	}

	asyncTask := func(t *task.Task) {
		t.UpdateStatus("started the task for AcceptNode: %s", t.ID)
		// Process the request
		if err := acceptNode(w, hostname, request.SaltFingerprint, t); err != nil {
			t.UpdateStatus("Failed")
		} else {
			t.UpdateStatus("Success")
		}
		t.Done()
	}
	if taskId, err := task.GetManager().Run("AcceptNode", asyncTask); err != nil {
		logger.Get().Error("Unable to create the task for AcceptNode", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed")

	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}

}

func acceptNode(w http.ResponseWriter, hostname string, fingerprint string, t *task.Task) error {

	if node, err := GetCoreNodeManager().AcceptNode(hostname, fingerprint); err == nil {
		t.UpdateStatus("Adding the node to DB: %s", hostname)
		if err = addStorageNodeToDB(w, *node); err != nil {
			t.UpdateStatus("Unable to add the node to DB: %s", hostname)
			return err
		}
	} else {
		logger.Get().Critical("Accepting the node failed: ", hostname)
		t.UpdateStatus("Unable to Accept the node: %s", hostname)
		return err
	}
	return nil
}

func addAndAcceptNode(w http.ResponseWriter, request models.AddStorageNodeRequest, t *task.Task) error {

	t.UpdateStatus("Bootstrapping the node")
	// Add the node
	if node, err := GetCoreNodeManager().AddNode(
		curr_hostname,
		request.Hostname,
		uint(request.SshPort),
		request.SshFingerprint,
		request.User,
		request.Password); err == nil {
		t.UpdateStatus("Adding the node to DB: %s", request.Hostname)
		if err = addStorageNodeToDB(w, *node); err != nil {
			t.UpdateStatus("Unable to add the node to DB: %s", request.Hostname)
			return err
		}
	} else {
		logger.Get().Critical("Bootstrapping the node failed: ", request.Hostname)
		t.UpdateStatus("Unable to Bootstrap the node: %s", request.Hostname)
		return err
	}
	return nil
}

func addStorageNodeToDB(w http.ResponseWriter, storage_node models.StorageNode) error {
	// Add the node details to the DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Insert(storage_node); err != nil {
		logger.Get().Critical("Error adding the node: %v", err)
		return err
	}
	return nil
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
			logger.Get().Error("Error getting the nodes list: %v", err)
			return
		}
	} else {
		if err := collection.Find(nil).All(&nodes); err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			logger.Get().Error("Error getting the nodes list: %v", err)
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
		logger.Get().Error("Error getting the node detail: %v", err)
	}

	if node.Hostname == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Node not found")
		logger.Get().Error("Node not found: %v", err)
		return
	} else {
		json.NewEncoder(w).Encode(node)
	}
}

func GET_UnmanagedNodes(w http.ResponseWriter, r *http.Request) {
	if nodes, err := GetCoreNodeManager().GetUnmanagedNodes(); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("Node not found: %v", err)
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
		logger.Get().Error("Error getting the node detail: %v", err)
	}

	return node
}
