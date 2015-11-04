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
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
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

	// Check if node already added
	// No need to check for error, as node would be nil in case of error and the same is checked
	if node, _ := node_exists("hostname", request.Hostname); node != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Node already added")
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
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	// Check if node already added
	// No need to check for error, as node would be nil in case of error and the same is checked
	if node, _ := node_exists("hostname", hostname); node != nil {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Node already added")
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

func addStorageNodeToDB(w http.ResponseWriter, storage_node models.Node) error {
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

func node_exists(key string, value string) (*models.Node, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
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
	admin_state_str := params.Get("state")

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if admin_state_str == "" {
		if err := collection.Find(nil).All(&nodes); err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			logger.Get().Error("Error getting the nodes list: %v", err)
			return
		}
	} else {
		nodes, err = getNodesWithState(w, admin_state_str)
		if err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			logger.Get().Error("Error getting the nodes list: %v", err)
			return
		}
	}
	if len(nodes) == 0 {
		json.NewEncoder(w).Encode([]models.Node{})
	} else {
		json.NewEncoder(w).Encode(nodes)
	}
}

func (a *App) GET_Node(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s", node_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
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
		logger.Get().Error("Nodes not found: %v", err)
	} else {
		if nodes == nil || len(*nodes) == 0 {
			json.NewEncoder(w).Encode([]models.Node{})
		} else {
			json.NewEncoder(w).Encode(nodes)
		}
	}
}

func GetNode(node_id uuid.UUID) models.Node {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": node_id}).One(&node); err != nil {
		logger.Get().Error("Error getting the node detail: %v", err)
	}

	return node
}

func getNodesWithState(w http.ResponseWriter, state string) (models.Nodes, error) {
	var validStates = [...]string{"free", "used", "unmanaged"}
	var found = false
	var foundIndex = -1
	for index, value := range validStates {
		if state == value {
			found = true
			foundIndex = index
			break
		}
	}

	if !found {
		return models.Nodes{}, errors.New(fmt.Sprintf("Invalid state value: %s", state))
	} else {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
		var nodes models.Nodes
		switch foundIndex {
		case 0:
			if err := coll.Find(bson.M{}).All(&nodes); err != nil {
				return models.Nodes{}, err
			}
			var unusedNodes models.Nodes
			for _, node := range nodes {
				if node.ClusterId.IsZero() {
					unusedNodes = append(unusedNodes, node)
				}
			}
			return unusedNodes, nil
		case 1:
			if err := coll.Find(bson.M{}).All(&nodes); err != nil {
				return models.Nodes{}, err
			}
			var usedNodes models.Nodes
			for _, node := range nodes {
				if !node.ClusterId.IsZero() {
					usedNodes = append(usedNodes, node)
				}
			}
			return usedNodes, nil
		case 2:
			if err := coll.Find(bson.M{"enabled": true}).All(&nodes); err != nil {
				return models.Nodes{}, err
			}
			return nodes, nil
		}
		return models.Nodes{}, nil
	}
}

func removeNode(w http.ResponseWriter, nodeId uuid.UUID) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	// Check if the node is free. If so remove the node
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": nodeId}).One(&node); err != nil {
		return false, errors.New("Unable to get node")
	}
	if !node.ClusterId.IsZero() {
		return false, errors.New("Node(s) participating in a cluster. Cannot be removed")
	}
	ret_val, err := GetCoreNodeManager().RemoveNode(node.Hostname)
	if ret_val {
		if err := collection.Remove(bson.M{"nodeid": nodeId}); err != nil {
			return false, errors.New("Error deleting the node(s) from DB")
		}
	} else {
		return false, err
	}
	return true, nil
}

func DELETE_Node(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s", node_id_str))
		return
	}

	if ok, err := removeNode(w, *node_id); err != nil || !ok {
		util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error removing the node: %v", err))
		return
	}
}

func DELETE_Nodes(w http.ResponseWriter, r *http.Request) {
	var nodeIds []struct {
		NodeId string `json:"nodeid"`
	}

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &nodeIds); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request"))
		return
	}

	for _, item := range nodeIds {
		node_id, _ := uuid.Parse(item.NodeId)
		if ok, err := removeNode(w, *node_id); err != nil || !ok {
			util.HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error removing the node(s): %v", err))
			return
		}
	}
}
