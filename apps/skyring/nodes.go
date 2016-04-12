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
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring-common/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var (
	curr_hostname, err = os.Hostname()
)

func (a *App) POST_Nodes(w http.ResponseWriter, r *http.Request) {
	var request models.AddStorageNodeRequest
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err), ctxt)
		if err := logAuditEvent(EventTypes["NODE_ADD_AND_ACCEPT"],
			fmt.Sprintf("Failed to add and accept node"),
			fmt.Sprintf("Failed to add and accept node. Error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add node event. Error: %v", ctxt, err)
		}
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err), ctxt)
		if err := logAuditEvent(EventTypes["NODE_ADD_AND_ACCEPT"],
			fmt.Sprintf("Failed to add and accept node"),
			fmt.Sprintf("Failed to add and accept node. Error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add node event. Error: %v", ctxt, err)
		}
		return
	}

	if request.SshPort == 0 {
		request.SshPort = models.DEFAULT_SSH_PORT
	}

	// Check if node already added
	// No need to check for error, as node would be nil in case of error and the same is checked
	if node, _ := node_exists("hostname", request.Hostname); node != nil {
		logger.Get().Error("%s-Node:%s already added", ctxt, request.Hostname)
		HttpResponse(w, http.StatusMethodNotAllowed, "Node already added", ctxt)
		if err := logAuditEvent(EventTypes["NODE_ADD_AND_ACCEPT"],
			fmt.Sprintf("Failed to add and accept node: %s", request.Hostname),
			fmt.Sprintf("Failed to add and accept node: %s. Error: %v", request.Hostname,
				fmt.Errorf("Node already added")),
			&node.NodeId,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add node event. Error: %v", ctxt, err)
		}
		return
	}

	// Validate for required fields
	if request.Hostname == "" || request.SshFingerprint == "" || request.User == "" || request.Password == "" {
		logger.Get().Error("%s-Required field(s) not provided", ctxt)
		HttpResponse(w, http.StatusBadRequest, "Required field(s) not provided", ctxt)
		if err := logAuditEvent(EventTypes["NODE_ADD_AND_ACCEPT"],
			fmt.Sprintf("Failed to add and accept node: %s", request.Hostname),
			fmt.Sprintf("Failed to add and accept node: %s. Error: %v", request.Hostname,
				fmt.Errorf("Required fields not provided")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add node event. Error: %v", ctxt, err)
		}
		return
	}

	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("started the task for addAndAcceptNode: %s", t.ID)
				var nodeId uuid.UUID
				appLock, err := lockNode(ctxt, nodeId, request.Hostname, "addAndAcceptNode")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["NODE_ADD_AND_ACCEPT"],
						fmt.Sprintf("Failed to add and accept node: %s", request.Hostname),
						fmt.Sprintf("Failed to add and accept node: %s. Error: %v", request.Hostname,
							err),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_HOST,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log add node event. Error: %v", ctxt, err)
					}
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)
				// Process the request
				if err := addAndAcceptNode(w, request, t, ctxt); err != nil {
					t.UpdateStatus("Failed")
					t.Done(models.TASK_STATUS_FAILURE)
					if err := logAuditEvent(EventTypes["NODE_ADD_AND_ACCEPT"],
						fmt.Sprintf("Failed to add and accept node: %s", request.Hostname),
						fmt.Sprintf("Failed to add and accept node: %s. Error: %v", request.Hostname,
							err),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_HOST,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log add node event. Error: %v", ctxt, err)
					}
				} else {
					t.UpdateStatus("Success")
					t.Done(models.TASK_STATUS_SUCCESS)
					if err := logAuditEvent(EventTypes["NODE_ADD_AND_ACCEPT"],
						fmt.Sprintf("Node: %s added and accepted successfully", request.Hostname),
						fmt.Sprintf("Node: %s added and accepted successfully", request.Hostname),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_HOST,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log add node event. Error: %v", ctxt, err)
					}

				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Add and Accept Node: %s", request.Hostname),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create the task for Add and Accept Node: %s. error: %v", ctxt, request.Hostname, err)
		HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed", ctxt)
		if err := logAuditEvent(EventTypes["NODE_ADD_AND_ACCEPT"],
			fmt.Sprintf("Failed to add and accept node: %s", request.Hostname),
			fmt.Sprintf("Failed to add and accept node: %s. Error: %v", request.Hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add node event. Error: %v", ctxt, err)
		}
	} else {
		logger.Get().Debug("Task Created: ", taskId.String(), ctxt)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) POST_AcceptUnamangedNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	var request models.UnmanagedNode

	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request. error: %v", err), ctxt)
		if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
			fmt.Sprintf("Failed to accept node %s", hostname),
			fmt.Sprintf("Failed to accept node %s. Error: %v", hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
		}
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err), ctxt)
		if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
			fmt.Sprintf("Failed to accept node %s", hostname),
			fmt.Sprintf("Failed to accept node %s. Error: %v", hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
		}
		return
	}

	// Check unmanaged node is present in DB
	if _, err := unaccepted_node_exists("hostname", hostname); err == mgo.ErrNotFound {
		logger.Get().Error("%s-Node:%s is not found", ctxt, hostname)
		HttpResponse(w, http.StatusMethodNotAllowed, "Node not found", ctxt)
		if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
			fmt.Sprintf("Failed to accept node %s", hostname),
			fmt.Sprintf("Failed to accept node %s. Error: %v", hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
		}
		return
	} else if err != nil {
		logger.Get().Error("%s-Error in retriving node %s", ctxt, hostname)
		HttpResponse(w, http.StatusMethodNotAllowed, "Error in retriving node", ctxt)
		if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
			fmt.Sprintf("Failed to accept node %s", hostname),
			fmt.Sprintf("Failed to accept node %s. Error: %v", hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
		}
		return
	}
	// Validate for required fields
	if hostname == "" || request.SaltFingerprint == "" {
		logger.Get().Error("%s-Required field(s) not provided", ctxt)
		HttpResponse(w, http.StatusBadRequest, "Required field(s) not provided", ctxt)
		if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
			fmt.Sprintf("Failed to accept node %s", hostname),
			fmt.Sprintf("Failed to accept node %s. Error: %v", hostname,
				fmt.Errorf("Required filed(s) not provided")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
		}
		return
	}

	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("started the task for AcceptNode: %s", t.ID)
				var nodeId uuid.UUID
				appLock, err := lockNode(ctxt, nodeId, hostname, "AcceptNode")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
						fmt.Sprintf("Failed to accept node %s", hostname),
						fmt.Sprintf("Failed to accept node %s. Error: %v", hostname,
							err),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_HOST,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
					}
					return
				}
				defer a.GetLockManager().ReleaseLock(ctxt, *appLock)
				// Process the request
				if err := acceptNode(w, hostname, request.SaltFingerprint, t, ctxt); err != nil {
					t.UpdateStatus("Failed")
					t.Done(models.TASK_STATUS_FAILURE)
					if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
						fmt.Sprintf("Failed to accept node %s", hostname),
						fmt.Sprintf("Failed to accept node %s. Error: %v", hostname,
							err),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_HOST,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
					}
				} else {
					t.UpdateStatus("Success")
					t.Done(models.TASK_STATUS_SUCCESS)
					if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
						fmt.Sprintf("Node %s accepted successfully", hostname),
						fmt.Sprintf("Node %s accepted successfully", hostname),
						nil,
						nil,
						models.NOTIFICATION_ENTITY_HOST,
						&(t.ID),
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
					}
				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Accept Node: %s", hostname),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create the task for Accept Node: %s. error: %v", ctxt, hostname, err)
		HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed", ctxt)
		if err := logAuditEvent(EventTypes["NODE_ACCEPT"],
			fmt.Sprintf("Failed to accept node %s", hostname),
			fmt.Sprintf("Failed to accept node %s. Error: %v", hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log accept node event. Error: %v", ctxt, err)
		}
	} else {
		logger.Get().Debug("%s-Task Created: %v", ctxt, taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		go Check_status(hostname, ctxt)
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func acceptNode(w http.ResponseWriter, hostname string, fingerprint string, t *task.Task, ctxt string) error {
	if _, err := GetCoreNodeManager().AcceptNode(hostname, fingerprint, ctxt); err == nil {
		t.UpdateStatus("Adding the node to DB: %s", hostname)
		if err = UpdateStorageNodeToDB(hostname, models.NODE_STATE_INITIALIZING, models.NODE_STATUS_UNKNOWN, models.ALARM_STATUS_CLEARED, ctxt); err != nil {
			logger.Get().Error("%s-Unable to add the node:%s to DB. error: %v", ctxt, hostname, err)
			t.UpdateStatus("Unable to add the node:%s to DB. error: %v", hostname, err)
			return err
		}
	} else {
		logger.Get().Critical("%s-Accepting the node: %s failed. error: %v", ctxt, hostname, err)
		t.UpdateStatus("Accepting the node: %s failed. error: %v", hostname, err)
		return err
	}
	return nil
}

func addAndAcceptNode(w http.ResponseWriter, request models.AddStorageNodeRequest, t *task.Task, ctxt string) error {
	t.UpdateStatus("Bootstrapping the node")
	// Add the node
	if _, err := GetCoreNodeManager().AddNode(
		curr_hostname,
		request.Hostname,
		uint(request.SshPort),
		request.SshFingerprint,
		request.User,
		request.Password,
		ctxt); err == nil {
		t.UpdateStatus("Adding the node to DB: %s", request.Hostname)
		if err = AddStorageNodeToDB(request.Hostname, models.NODE_STATE_INITIALIZING, models.NODE_STATUS_UNKNOWN, models.ALARM_STATUS_CLEARED, ctxt); err != nil {
			logger.Get().Error("%s-Unable to add the node:%s to DB. error: %v", ctxt, request.Hostname, err)
			t.UpdateStatus("Unable to add the node:%s to DB. error: %v", request.Hostname, err)
			return err
		}

	} else {
		logger.Get().Critical("%s-Bootstrapping the node: %s failed. error: %v: ", ctxt, request.Hostname, err)
		t.UpdateStatus("Bootstrapping the node: %s failed. error: %v: ", request.Hostname, err)
		return err
	}
	return nil
}

func AddStorageNodeToDB(hostname string, node_state models.NodeState, node_status models.NodeStatus, alm_status models.AlarmStatus, ctxt string) error {
	// Add the node details to the DB
	var storage_node models.Node
	storage_node.Hostname = hostname
	storage_node.State = node_state
	storage_node.Status = node_status
	storage_node.AlmStatus = alm_status

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)

	var node models.Node
	err = coll.Find(bson.M{"hostname": storage_node.Hostname}).One(&node)
	if err != mgo.ErrNotFound {
		logger.Get().Critical(fmt.Sprintf("%s-Node with name: %v already exists", ctxt, storage_node.Hostname))
		return errors.New(fmt.Sprintf("Node with name: %v already exists", storage_node.Hostname))
	}

	// Persist the node details
	if err := coll.Insert(storage_node); err != nil {
		logger.Get().Critical("%s-Error adding the node: %s. error: %v", ctxt, storage_node.Hostname, err)
		return err
	}
	return nil
}

func node_exists(key string, value interface{}) (*models.Node, error) {
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
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	params := r.URL.Query()
	admin_state_str := params.Get("state")

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if admin_state_str == "" {
		if err := collection.Find(bson.M{"state": bson.M{"$ne": models.NODE_STATE_UNACCEPTED}}).All(&nodes); err != nil {
			HttpResponse(w, http.StatusInternalServerError, err.Error())
			logger.Get().Error("%s-Error getting the nodes list. error: %v", ctxt, err)
			return
		}
	} else {
		nodes, err = getNodesWithState(w, admin_state_str)
		if err != nil {
			HttpResponse(w, http.StatusInternalServerError, err.Error())
			logger.Get().Error("%s-Error getting the nodes list. error: %v", ctxt, err)
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
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing node id: %s", ctxt, node_id_str)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s", node_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
		logger.Get().Error("%s-Error getting the node detail for %v. error: %v", ctxt, *node_id, err)
	}

	if node.Hostname == "" {
		HttpResponse(w, http.StatusBadRequest, "Node not found")
		logger.Get().Error("%s-Node: %v not found. error: %v", ctxt, *node_id, err)
		return
	} else {
		json.NewEncoder(w).Encode(node)
	}
}

func (a *App) GET_UnmanagedNodes(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	var nodes models.Nodes
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"state": models.NODE_STATE_UNACCEPTED}).All(&nodes); err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("%s-No un-managed nodes found. error: %v", ctxt, err)
	} else {
		if len(nodes) == 0 {
			json.NewEncoder(w).Encode([]models.Node{})
		} else {
			json.NewEncoder(w).Encode(nodes)
		}
	}
}

func GetNode(node_id uuid.UUID) (node models.Node, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := collection.Find(bson.M{"nodeid": node_id}).One(&node); err != nil {
		return node, fmt.Errorf("Error getting the detail of node: %v. error: %v", node_id, err)
	}

	return node, nil
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
			if err := coll.Find(bson.M{"enabled": false}).All(&nodes); err != nil {
				return models.Nodes{}, err
			}
			return nodes, nil
		}
		return models.Nodes{}, nil
	}
}

func removeNode(ctxt string, w http.ResponseWriter, nodeId uuid.UUID, t *task.Task) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	// Check if the node is free. If so remove the node
	t.UpdateStatus("Getting node details from DB")
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": nodeId}).One(&node); err != nil {
		return false, errors.New("Unable to get node")
	}
	if !node.ClusterId.IsZero() {
		return false, errors.New("Node(s) participating in a cluster. Cannot be removed")
	}
	appLock, err := lockNode(ctxt, node.NodeId, node.Hostname, "addAndAcceptNode")
	if err != nil {
		return false, err
	}
	defer GetApp().GetLockManager().ReleaseLock(ctxt, *appLock)
	t.UpdateStatus("Running backend removal of node")
	ret_val, err := GetCoreNodeManager().RemoveNode(node.Hostname, ctxt)
	if ret_val {
		t.UpdateStatus("Removing node from DB")
		if err := collection.Remove(bson.M{"nodeid": nodeId}); err != nil {
			return false, errors.New("Error deleting the node(s) from DB")
		}
	} else {
		return false, err
	}
	t.UpdateStatus(fmt.Sprintf("Node: %v removed", nodeId))
	return true, nil
}

func (a *App) DELETE_Node(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing node id: %s. error: %v", ctxt, node_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s", node_id_str))
		if err := logAuditEvent(EventTypes["NODE_DELETE"],
			fmt.Sprintf("Failed to delete node"),
			fmt.Sprintf("Failed to delete node. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
		}
		return
	}
	nodeName, err := getNodeNameFromId(*node_id)
	if err != nil || nodeName == "" {
		nodeName = node_id_str
	}
	asyncTask := func(t *task.Task) {
		t.UpdateStatus("Started the task for remove node: %v", t.ID)
		if ok, err := removeNode(ctxt, w, *node_id, t); err != nil || !ok {
			util.FailTask(fmt.Sprintf("Error removing the node: %v", *node_id), fmt.Errorf("%s-%v", ctxt, err), t)
			if err := logAuditEvent(EventTypes["NODE_DELETE"],
				fmt.Sprintf("Failed to delete node %s", nodeName),
				fmt.Sprintf("Failed to delete node %s. Error: %v", nodeName,
					err),
				node_id,
				nil,
				models.NOTIFICATION_ENTITY_HOST,
				&(t.ID),
				ctxt); err != nil {
				logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
			}
			return
		}
		t.UpdateStatus("Success")
		t.Done(models.TASK_STATUS_SUCCESS)
		if err := logAuditEvent(EventTypes["NODE_DELETE"],
			fmt.Sprintf("Node %s deleted successfully", nodeName),
			fmt.Sprintf("Node %s deleted successfully", nodeName),
			node_id,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			&(t.ID),
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		fmt.Sprintf("Remove Node: %v", *node_id),
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create the task for remove node", ctxt, err)
		HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed")
		if err := logAuditEvent(EventTypes["NODE_DELETE"],
			fmt.Sprintf("Failed to delete node %s", nodeName),
			fmt.Sprintf("Failed to delete node %s. Error: %v", nodeName,
				err),
			node_id,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
		}

	} else {
		logger.Get().Debug("%s-Task Created: ", ctxt, taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) DELETE_Nodes(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	var nodeIds []struct {
		NodeId string `json:"nodeid"`
	}

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request. error: %v", err))
		if err := logAuditEvent(EventTypes["NODE_DELETE"],
			fmt.Sprintf("Failed to delete nodes"),
			fmt.Sprintf("Failed to delete nodes. Error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
		}
		return
	}
	if err := json.Unmarshal(body, &nodeIds); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		if err := logAuditEvent(EventTypes["NODE_DELETE"],
			fmt.Sprintf("Failed to delete nodes"),
			fmt.Sprintf("Failed to delete nodes. Error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
		}
		return
	}
	nodeNameList := ""
	var failedNodes []string
	asyncTask := func(t *task.Task) {
		t.UpdateStatus("Started the task for remove multiple nodes: %v", t.ID)
		for _, item := range nodeIds {
			node_id, err := uuid.Parse(item.NodeId)
			nodeName, err := getNodeNameFromId(*node_id)
			if err != nil || nodeName == "" {
				nodeName = item.NodeId
			}
			if err != nil {
				failedNodes = append(failedNodes, item.NodeId)
				if err := logAuditEvent(EventTypes["NODE_DELETE"],
					fmt.Sprintf("Failed to delete node: %s", nodeName),
					fmt.Sprintf("Failed to delete node: %s. Error: %v", nodeName, err),
					node_id,
					nil,
					models.NOTIFICATION_ENTITY_HOST,
					&(t.ID),
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
				}
				continue
			}
			t.UpdateStatus("Removing node: %v", *node_id)
			if ok, err := removeNode(ctxt, w, *node_id, t); err != nil || !ok {
				util.FailTask(fmt.Sprintf("Error removing node: %v", *node_id), fmt.Errorf("%s-%v", ctxt, err), t)
				failedNodes = append(failedNodes, item.NodeId)
				if err := logAuditEvent(EventTypes["NODE_DELETE"],
					fmt.Sprintf("Failed to delete node: %s", nodeName),
					fmt.Sprintf("Failed to delete node: %s. Error: %v", nodeName,
						fmt.Errorf("Error while removing the node")),
					node_id,
					nil,
					models.NOTIFICATION_ENTITY_HOST,
					&(t.ID),
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
				}
				continue
			}
			if nodeNameList == "" {
				nodeNameList += fmt.Sprintf("%s", nodeName)
			} else {
				nodeNameList += fmt.Sprintf(", %s", nodeName)
			}

		}
		if len(failedNodes) > 0 {
			t.UpdateStatus("Failed to remove the node id(s): %v", failedNodes)
		}
		t.UpdateStatus("Success")
		t.Done(models.TASK_STATUS_SUCCESS)
		if err := logAuditEvent(EventTypes["NODE_DELETE"],
			fmt.Sprintf("Deleted nodes: %s successfully", nodeNameList),
			fmt.Sprintf("Deleted nodes: %s successfully", nodeNameList),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			&(t.ID),
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
		}

	}
	if taskId, err := a.GetTaskManager().Run(
		models.ENGINE_NAME,
		"Remove Multiple Nodes",
		asyncTask,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error("%s-Unable to create the task for remove multiple nodes", ctxt, err)
		HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed")
		if err := logAuditEvent(EventTypes["NODE_DELETE"],
			fmt.Sprintf("Failed to delete nodes"),
			fmt.Sprintf("Failed to delete nodes. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
		}

	} else {
		logger.Get().Debug("%s-Task Created: ", ctxt, taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) GET_Disks(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing node-id: %s. error: %v", ctxt, node_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s. Error: %v", node_id_str, err))
		return
	}
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting the node detail for node: %s. error: %v", ctxt, node_id_str, err))
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(node.StorageDisks); err != nil {
		logger.Get().Error("%s-Error encoding the data: %v", ctxt, err)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) GET_Disk(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing node-id: %s. error: %v", ctxt, node_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s. Error: %v", node_id_str, err))
		return
	}

	disk_id_str := vars["disk-id"]
	disk_id, err := uuid.Parse(disk_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing disk id: %s. error: %v", ctxt, disk_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing disk id: %s. Error: %v", disk_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting the node detail for node: %s. error: %v", ctxt, node_id_str, err))
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	var mdisk models.Disk
	for _, disk := range node.StorageDisks {
		if disk.DiskId == *disk_id {
			mdisk = disk
			break
		}
	}
	if err := json.NewEncoder(w).Encode(mdisk); err != nil {
		logger.Get().Error("%s-Error encoding data: %v", ctxt, err)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
	}

}

func (a *App) PATCH_Disk(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing node id: %s. error: %v", ctxt, node_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s. Error: %v", node_id_str, err))
		if err := logAuditEvent(EventTypes["UPDATE_DISK"],
			fmt.Sprintf("Failed to update disks"),
			fmt.Sprintf("Failed to update disks. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update disk event. Error: %v", ctxt, err)
		}

		return
	}
	nodeName, err := getNodeNameFromId(*node_id)
	if err != nil || nodeName == "" {
		nodeName = node_id_str
	}

	disk_id_str := vars["disk-id"]
	disk_id, err := uuid.Parse(disk_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing disk id: %s. error: %v", ctxt, disk_id_str, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing disk id: %s. Error: %v", disk_id_str, err))
		if err := logAuditEvent(EventTypes["UPDATE_DISK"],
			fmt.Sprintf("Failed to update disks for node: %s", nodeName),
			fmt.Sprintf("Failed to update disks for node: %s. Error: %v", nodeName,
				err),
			node_id,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update disk event. Error: %v", ctxt, err)
		}

		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting the node detail for node: %s. error: %v", ctxt, node_id_str, err))
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		if err := logAuditEvent(EventTypes["UPDATE_DISK"],
			fmt.Sprintf("Failed to update disks for node: %s", nodeName),
			fmt.Sprintf("Failed to update disks for node: %s. Error: %v", nodeName,
				err),
			node_id,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update disk event. Error: %v", ctxt, err)
		}
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body: %s", ctxt, err)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		if err := logAuditEvent(EventTypes["UPDATE_DISK"],
			fmt.Sprintf("Failed to update disks for node: %s", nodeName),
			fmt.Sprintf("Failed to update disks for node: %s. Error: %v", nodeName,
				err),
			node_id,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update disk event. Error: %v", ctxt, err)
		}
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data: %s", ctxt, err)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		if err := logAuditEvent(EventTypes["UPDATE_DISK"],
			fmt.Sprintf("Failed to update disks for node: %s", nodeName),
			fmt.Sprintf("Failed to update disks for node: %s . Error: %v", nodeName,
				err),
			node_id,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update disk event. Error: %v", ctxt, err)
		}
		return
	}

	var disks []models.Disk
	if val, ok := m["storageprofile"]; ok {
		//update the field
		for _, disk := range node.StorageDisks {
			if disk.DiskId == *disk_id {
				disk.StorageProfile = val.(string)
			}
			disks = append(disks, disk)
		}
		node.StorageDisks = disks
	}
	//Save
	err = collection.Update(bson.M{"nodeid": *node_id}, bson.M{"$set": node})
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error updating record in DB for node: %s. error: %v", ctxt, node_id_str, err))
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		if err := logAuditEvent(EventTypes["UPDATE_DISK"],
			fmt.Sprintf("Failed to update disks for node: %s", nodeName),
			fmt.Sprintf("Failed to update disks for node: %s. Error: %v", nodeName,
				err),
			node_id,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update disk event. Error: %v", ctxt, err)
		}
	}
}

func (a *App) POST_Actions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error parsing http request body.error: %v", ctxt, err))
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
			fmt.Sprintf("Failed to modify node: %s", hostname),
			fmt.Sprintf("Failed to modify node: %s. Error: %v", hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log modify node event. Error: %v", ctxt, err)
		}
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Unable to Unmarshall the data.error: %v", ctxt, err))
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
			fmt.Sprintf("Failed to modify node: %s", hostname),
			fmt.Sprintf("Failed to modify node: %s. Error: %v", hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log modify node event. Error: %v", ctxt, err)
		}
		return
	}
	node, err := node_exists("hostname", hostname)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Node %s not found . error: %v", ctxt, hostname, err))
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
			fmt.Sprintf("Failed to modify node: %s", hostname),
			fmt.Sprintf("Failed to modify node: %s. Error: %v", hostname,
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_HOST,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log modify node event. Error: %v", ctxt, err)
		}
		return
	}
	ch := m["action"]
	switch ch {
	case "reinitialize":
		{
			if node.State != models.NODE_STATE_FAILED {
				logger.Get().Error(fmt.Sprintf("%s-Node %s is not in failed state", ctxt, hostname))
				HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Node %s is not in failed state", hostname))
				if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
					fmt.Sprintf("Failed to reinitialize node: %s", hostname),
					fmt.Sprintf("Failed to reinitialize node: %s. Error: %v", hostname,
						fmt.Errorf("Node not in failed state")),
					&(node.NodeId),
					nil,
					models.NOTIFICATION_ENTITY_HOST,
					nil,
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log reinitialize node event. Error: %v", ctxt, err)
				}
				return
			}
			if ok, err := GetCoreNodeManager().IsNodeUp(node.Hostname, ctxt); !ok {
				logger.Get().Error(fmt.Sprintf("%s-Error getting status of node: %s. error: %v", ctxt, hostname, err))
				HttpResponse(w, http.StatusInternalServerError,
					fmt.Sprintf("Error getting status of node: %s. error: %v", hostname, err))
				if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
					fmt.Sprintf("Failed to reinitialize node: %s", hostname),
					fmt.Sprintf("Failed to reinitialize node: %s. Error: %v", hostname,
						fmt.Errorf("Error getting node status")),
					&(node.NodeId),
					nil,
					models.NOTIFICATION_ENTITY_HOST,
					nil,
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log reinitialize node event. Error: %v", ctxt, err)
				}
				return
			}
			sessionCopy := db.GetDatastore().Copy()
			defer sessionCopy.Close()
			coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
			if err := coll.Update(bson.M{"hostname": node.Hostname},
				bson.M{"$set": bson.M{"state": models.NODE_STATE_INITIALIZING}}); err != nil {
				logger.Get().Error(fmt.Sprintf("%s-Failed to update the node %s state as initialize: error: %v", ctxt, hostname, err))
				HttpResponse(w, http.StatusInternalServerError,
					fmt.Sprintf("Failed to update the node %s state as initialize: error: %v", hostname, err))
				if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
					fmt.Sprintf("Failed to reinitialize node: %s", hostname),
					fmt.Sprintf("Failed to reinitialize node: %s. Error: %v", hostname,
						fmt.Errorf("Error updating node state to initializing state")),
					&(node.NodeId),
					nil,
					models.NOTIFICATION_ENTITY_HOST,
					nil,
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log reinitialize node event. Error: %v", ctxt, err)
				}
				return
			}
			Initialize(node.Hostname, ctxt)
		}
	case "delete":
		{
			sessionCopy := db.GetDatastore().Copy()
			defer sessionCopy.Close()
			collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
			var node models.Node
			if err := collection.Find(bson.M{"hostname": hostname}).One(&node); err != nil {
				logger.Get().Error(fmt.Sprintf("%s-Unable to get node details of %s : error: %v", ctxt, hostname, err))
				HttpResponse(w, http.StatusInternalServerError,
					fmt.Sprintf("Failed to get node details of %s : error: %v", hostname, err))
				if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
					fmt.Sprintf("Failed to delete node: %s", hostname),
					fmt.Sprintf("Failed to delete node: %s. Error: %v", hostname,
						err),
					&(node.NodeId),
					nil,
					models.NOTIFICATION_ENTITY_HOST,
					nil,
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
				}
				return
			}
			if !node.ClusterId.IsZero() {
				logger.Get().Error(fmt.Sprintf("%s-Host %s already participating in a cluster, Cannot be removed: error: %v",
					ctxt, hostname, err))
				HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete host %s : error: %v", hostname, err))
				if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
					fmt.Sprintf("Failed to delete node: %s", hostname),
					fmt.Sprintf("Failed to delete node: %s. Error: %v", hostname,
						fmt.Errorf("Node is participating in a cluster")),
					&(node.NodeId),
					nil,
					models.NOTIFICATION_ENTITY_HOST,
					nil,
					ctxt); err != nil {
					logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
				}
				return
			}
			if ret_val, _ := GetCoreNodeManager().RemoveNode(node.Hostname, ctxt); ret_val {
				if err := collection.Remove(bson.M{"hostname": node.Hostname}); err != nil {
					logger.Get().Error(fmt.Sprintf("%s-Error removing Host %s from DB: error: %v", ctxt, hostname, err))
					HttpResponse(w, http.StatusInternalServerError,
						fmt.Sprintf("Error removing host %s from DB: error: %v", hostname, err))
					if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
						fmt.Sprintf("Failed to delete node: %s", hostname),
						fmt.Sprintf("Failed to delete node: %s. Error: %v", hostname,
							fmt.Errorf("Error removing the DB entry")),
						&(node.NodeId),
						nil,
						models.NOTIFICATION_ENTITY_HOST,
						nil,
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log delete node event. Error: %v", ctxt, err)
					}
					return
				} else {
					if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
						fmt.Sprintf("Node: %s deleted successfully", hostname),
						fmt.Sprintf("Node: %s deleted successfully", hostname),
						&(node.NodeId),
						nil,
						models.NOTIFICATION_ENTITY_HOST,
						nil,
						ctxt); err != nil {
						logger.Get().Error("%s- Unable to log modify node event. Error: %v", ctxt, err)
					}
				}
			}
		}
	default:
		{
			logger.Get().Error(fmt.Sprintf("%s-Unsupported action request found for Node:%s", ctxt, hostname))
			HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Unsupported action request found for Node:%s", ch))
			if err := logAuditEvent(EventTypes["NODE_MODIFIED"],
				fmt.Sprintf("Failed to modify node: %s", hostname),
				fmt.Sprintf("Failed to modify node: %s. Error: %v", hostname,
					fmt.Errorf("Unsupported action")),
				&(node.NodeId),
				nil,
				models.NOTIFICATION_ENTITY_HOST,
				nil,
				ctxt); err != nil {
				logger.Get().Error("%s- Unable to log modify node event. Error: %v", ctxt, err)
			}
			return
		}
	}
}

func unaccepted_node_exists(key string, value string) (*models.Node, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{key: value, "state": models.NODE_STATE_UNACCEPTED}).One(&node); err == nil {
		return &node, nil
	} else {
		return nil, err
	}
}

func UpdateStorageNodeToDB(hostname string, node_state models.NodeState, node_status models.NodeStatus, alm_status models.AlarmStatus, ctxt string) error {
	// Updating the node details in DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": hostname}, bson.M{"$set": bson.M{"state": node_state, "status": node_status, "almstatus": alm_status}}); err != nil {
		logger.Get().Critical("%s-Error Updating the node: %s. error: %v", ctxt, hostname, err)
		return err
	}
	return nil
}

func Check_status(hostname string, ctxt string) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	node := new(models.Node)
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	for i := 0; i < 20; i++ {
		time.Sleep(30 * time.Second)
		if err := collection.Find(bson.M{"hostname": hostname, "state": models.NODE_STATE_ACTIVE}).One(&node); err == nil {
			return
		}
	}
	if ok, err := GetCoreNodeManager().IsNodeUp(hostname, ctxt); !ok {
		logger.Get().Error(fmt.Sprintf("%s-Error getting status of node: %s. error: %v", ctxt, hostname, err))
		if err := collection.Update(bson.M{"hostname": hostname}, bson.M{"$set": bson.M{"state": models.NODE_STATE_FAILED}}); err != nil {
			logger.Get().Critical("%s-Error Updating the node: %s. error: %v", ctxt, hostname, err)
		}
		return
	}
	Initialize(hostname, ctxt)
}

func getNodeNameFromId(uuid uuid.UUID) (string, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": uuid}).One(&node); err == nil {
		return node.Hostname, nil
	} else {
		return "", err
	}
}
