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

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request. error: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Unable to unmarshal request. error: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}

	if request.SshPort == 0 {
		request.SshPort = models.DEFAULT_SSH_PORT
	}

	// Check if node already added
	// No need to check for error, as node would be nil in case of error and the same is checked
	if node, _ := node_exists("hostname", request.Hostname); node != nil {
		logger.Get().Error("Node:%s already added", request.Hostname)
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Node already added")
		return
	}

	// Validate for required fields
	if request.Hostname == "" || request.SshFingerprint == "" || request.User == "" || request.Password == "" {
		logger.Get().Error("Required field(s) not provided")
		util.HttpResponse(w, http.StatusBadRequest, "Required field(s) not provided")
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
				appLock, err := lockNode(nodeId, request.Hostname, "addAndAcceptNode")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				// Process the request
				if err := addAndAcceptNode(w, request, t); err != nil {
					t.UpdateStatus("Failed")
					t.Done(models.TASK_STATUS_FAILURE)
				} else {
					t.UpdateStatus("Success")
					t.Done(models.TASK_STATUS_SUCCESS)
				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Add and Accept Node: %s", request.Hostname), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create the task for Add and Accept Node: %s. error: %v", request.Hostname, err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed")
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) POST_AcceptUnamangedNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	var request models.UnmanagedNode

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request. error: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request. error: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Unable to unmarshal request. error: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	// Check if node already added
	// No need to check for error, as node would be nil in case of error and the same is checked
	if node, _ := node_exists("hostname", hostname); node != nil {
		logger.Get().Error("Node:%s already added", hostname)
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Node already added")
		return
	}

	// Validate for required fields
	if hostname == "" || request.SaltFingerprint == "" {
		logger.Get().Error("Required field(s) not provided")
		util.HttpResponse(w, http.StatusBadRequest, "Required field(s) not provided")
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
				appLock, err := lockNode(nodeId, hostname, "AcceptNode")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				// Process the request
				if err := acceptNode(w, hostname, request.SaltFingerprint, t); err != nil {
					t.UpdateStatus("Failed")
					t.Done(models.TASK_STATUS_FAILURE)
				} else {
					t.UpdateStatus("Success")
					t.Done(models.TASK_STATUS_SUCCESS)
				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Accept Node: %s", hostname), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create the task for Accept Node: %s. error: %v", hostname, err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed")
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func acceptNode(w http.ResponseWriter, hostname string, fingerprint string, t *task.Task) error {
	if _, err := GetCoreNodeManager().AcceptNode(hostname, fingerprint); err == nil {
		t.UpdateStatus("Adding the node to DB: %s", hostname)
		if err = AddStorageNodeToDB(hostname, models.NODE_STATE_INITIALIZING, models.STATUS_UP); err != nil {
			logger.Get().Error("Unable to add the node:%s to DB. error: %v", hostname, err)
			t.UpdateStatus("Unable to add the node:%s to DB. error: %v", hostname, err)
			return err
		}
	} else {
		logger.Get().Critical("Accepting the node: %s failed. error: %v", hostname, err)
		t.UpdateStatus("Accepting the node: %s failed. error: %v", hostname, err)
		return err
	}
	return nil
}

func addAndAcceptNode(w http.ResponseWriter, request models.AddStorageNodeRequest, t *task.Task) error {
	t.UpdateStatus("Bootstrapping the node")
	// Add the node
	if _, err := GetCoreNodeManager().AddNode(
		curr_hostname,
		request.Hostname,
		uint(request.SshPort),
		request.SshFingerprint,
		request.User,
		request.Password); err == nil {
		t.UpdateStatus("Adding the node to DB: %s", request.Hostname)
		if err = AddStorageNodeToDB(request.Hostname, models.NODE_STATE_INITIALIZING, models.STATUS_UP); err != nil {
			logger.Get().Error("Unable to add the node:%s to DB. error: %v", request.Hostname, err)
			t.UpdateStatus("Unable to add the node:%s to DB. error: %v", request.Hostname, err)
			return err
		}

	} else {
		logger.Get().Critical("Bootstrapping the node: %s failed. error: %v: ", request.Hostname, err)
		t.UpdateStatus("Bootstrapping the node: %s failed. error: %v: ", request.Hostname, err)
		return err
	}
	return nil
}

func AddStorageNodeToDB(hostname string, node_state int, node_status string) error {
	// Add the node details to the DB
	var storage_node models.Node
	storage_node.Hostname = hostname
	storage_node.State = node_state
	storage_node.Status = node_status

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)

	var node models.Node
	err = coll.Find(bson.M{"hostname": storage_node.Hostname}).One(&node)
	if err != mgo.ErrNotFound {
		logger.Get().Critical(fmt.Sprintf("Node with name: %v already exists", storage_node.Hostname))
		return errors.New(fmt.Sprintf("Node with name: %v already exists", storage_node.Hostname))
	}

	// Persist the node details
	if err := coll.Insert(storage_node); err != nil {
		logger.Get().Critical("Error adding the node: %s. error: %v", storage_node.Hostname, err)
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
			logger.Get().Error("Error getting the nodes list. error: %v", err)
			return
		}
	} else {
		nodes, err = getNodesWithState(w, admin_state_str)
		if err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			logger.Get().Error("Error getting the nodes list. error: %v", err)
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
		logger.Get().Error("Error parsing node id: %s", node_id_str)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s", node_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
		logger.Get().Error("Error getting the node detail for %v. error: %v", *node_id, err)
	}

	if node.Hostname == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Node not found")
		logger.Get().Error("Node: %v not found. error: %v", *node_id, err)
		return
	} else {
		json.NewEncoder(w).Encode(node)
	}
}

func (a *App) GET_UnmanagedNodes(w http.ResponseWriter, r *http.Request) {
	if nodes, err := GetCoreNodeManager().GetUnmanagedNodes(); err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		logger.Get().Error("No un-managed nodes found. error: %v", err)
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
		logger.Get().Error("Error getting the detail of node: %v. error: %v", node_id, err)
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
			if err := coll.Find(bson.M{"enabled": false}).All(&nodes); err != nil {
				return models.Nodes{}, err
			}
			return nodes, nil
		}
		return models.Nodes{}, nil
	}
}

func removeNode(w http.ResponseWriter, nodeId uuid.UUID, t *task.Task) (bool, error) {
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
	appLock, err := lockNode(node.NodeId, node.Hostname, "addAndAcceptNode")
	if err != nil {
		return false, err
	}
	defer GetApp().GetLockManager().ReleaseLock(*appLock)
	t.UpdateStatus("Running backend removal of node")
	ret_val, err := GetCoreNodeManager().RemoveNode(node.Hostname)
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
	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		logger.Get().Error("Error parsing node id: %s. error: %v", node_id_str, err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s", node_id_str))
		return
	}

	asyncTask := func(t *task.Task) {
		t.UpdateStatus("Started the task for remove node: %v", t.ID)
		if ok, err := removeNode(w, *node_id, t); err != nil || !ok {
			util.FailTask(fmt.Sprintf("Error removing the node: %v", *node_id), err, t)
			return
		}
		t.UpdateStatus("Success")
		t.Done(models.TASK_STATUS_SUCCESS)
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Remove Node: %v", *node_id), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create the task for remove node", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed")

	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) DELETE_Nodes(w http.ResponseWriter, r *http.Request) {
	var nodeIds []struct {
		NodeId string `json:"nodeid"`
	}

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request. error: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request. error: %v", err))
		return
	}
	if err := json.Unmarshal(body, &nodeIds); err != nil {
		logger.Get().Error("Unable to unmarshal request. error: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}
	var failedNodes []string
	asyncTask := func(t *task.Task) {
		t.UpdateStatus("Started the task for remove multiple nodes: %v", t.ID)
		for _, item := range nodeIds {
			node_id, err := uuid.Parse(item.NodeId)
			if err != nil {
				failedNodes = append(failedNodes, item.NodeId)
				continue
			}
			t.UpdateStatus("Removing node: %v", *node_id)
			if ok, err := removeNode(w, *node_id, t); err != nil || !ok {
				util.FailTask(fmt.Sprintf("Error removing node: %v", *node_id), err, t)
				failedNodes = append(failedNodes, item.NodeId)
			}
		}
		if len(failedNodes) > 0 {
			t.UpdateStatus("Failed to remove the node id(s): %v", failedNodes)
		}
		t.UpdateStatus("Success")
		t.Done(models.TASK_STATUS_SUCCESS)
	}
	if taskId, err := a.GetTaskManager().Run("Remove Multiple Nodes", asyncTask, 300*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create the task for remove multiple nodes", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Task Creation Failed")

	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) GET_Disks(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s. Error: %v", node_id_str, err))
		return
	}
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
		logger.Get().Error(fmt.Sprintf("Error getting the node detail for node: %s. error: %v", node_id_str, err))
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(node.StorageDisks); err != nil {
		logger.Get().Error("Error encoding the data: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) GET_Disk(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s. Error: %v", node_id_str, err))
		return
	}

	disk_id_str := vars["disk-id"]
	disk_id, err := uuid.Parse(disk_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing disk id: %s. Error: %v", disk_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
		logger.Get().Error(fmt.Sprintf("Error getting the node detail for node: %s. error: %v", node_id_str, err))
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
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
		logger.Get().Error("Error encoding data: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	}

}

func (a *App) PATCH_Disk(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, err := uuid.Parse(node_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s. Error: %v", node_id_str, err))
		return
	}

	disk_id_str := vars["disk-id"]
	disk_id, err := uuid.Parse(disk_id_str)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing node id: %s. Error: %v", disk_id_str, err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := collection.Find(bson.M{"nodeid": *node_id}).One(&node); err != nil {
		logger.Get().Error(fmt.Sprintf("Error getting the node detail for node: %s. error: %v", node_id_str, err))
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
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
		logger.Get().Error(fmt.Sprintf("Error updating record in DB for node: %s. error: %v", node_id_str, err))
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}
