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
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/utils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

type StorageNode struct {
	UUID              string
	Hostname          string
	SshFingerprint    string
	Tags              map[string]string
	MachineId         string
	ManagementIp      string
	ClusterIp         string
	PublicIp          string
	ClusterId         string
	Location          string
	Status            string
	Options           map[string]string
	State             string
	CPUs              []CPU
	NetworkInterfaces []NIC
	StorageDisks      []StorageDisk
	Memory            []Memory
	OS                OperatingSystem
	ManagedState      string
}

type AddStorageNodeRequest struct {
	Hostname       string
	SshFingerprint string
	User           string
	Password       string
	SshPort        int
}

type CPU struct {
	CPUId string
}

type NIC struct {
	Name    string
	Type    string
	Address string
	Subnet  string
	Gateway string
	VLanId  string
}

type StorageDisk struct {
	Name       string
	Speed      int
	Capacity   int
	FreeSize   int
	Type       string
	Latency    int
	Throughput int
	Location   string
}

type Memory struct {
	Name       string
	Type       string
	TotalSize  int
	FreeSize   int
	Attributes string
}

type OperatingSystem struct {
	Name                    string
	OSVersion               string
	KernelVersion           string
	StorageProviderVersion  string
	KdumpStatus             string
	MemoryPageSharingStatus string
	AutomaticLaregPages     bool
	SELinuxMode             string
}

type StorageNodes []StorageNode

var (
	curr_hostname, err = os.Hostname()
)

const (
	DEFAULT_SSH_PORT        = 22
	REQUEST_SIZE_LIMIT      = 1048576
	COLL_NAME_STORAGE_NODES = "storage_nodes"
	NODE_STATE_FREE         = "free"
	NODE_STATE_UNMANAGED    = "unmanaged"
	NODE_STATE_USED         = "used"
)

func Nodes_PUT(w http.ResponseWriter, r *http.Request) {
	var request AddStorageNodeRequest

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, REQUEST_SIZE_LIMIT))
	if err != nil {
		glog.Errorf("Error parsing the request: %v", err)
	}
	if err := r.Body.Close(); err != nil {
		glog.Errorf("Error parsing the request: %v", err)
	}
	if err := json.Unmarshal(body, &request); err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}

	var storage_node StorageNode

	storage_node.UUID = uuid.NewUUID().String()
	storage_node.Hostname = request.Hostname
	storage_node.SshFingerprint = request.SshFingerprint

	// Check if node already added
	if exists("hostname", request.Hostname) {
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Node already added")
		return
	}

	// Add the node
	ret_val := node_manager.AddNode(request.Hostname, storage_node.SshFingerprint, request.User, request.Password, curr_hostname)

	if ret_val == true {
		// Sleep for sometime to allow salt minoins to initialize
		time.Sleep(10 * time.Second)

		storage_node.ManagedState = NODE_STATE_FREE
		storage_node.MachineId = node_manager.GetNodeMachineId(request.Hostname)

		// Add the node details to the DB
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()

		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.AppDatabase).C(COLL_NAME_STORAGE_NODES)
		if err := coll.Insert(storage_node); err != nil {
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			glog.Fatalf("Error adding the node: %v", err)
			return
		}

		if err := json.NewEncoder(w).Encode("Added successfully"); err != nil {
			glog.Errorf("Error: %v", err)
		}
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}

func exists(key string, value string) bool {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.AppDatabase).C(COLL_NAME_STORAGE_NODES)
	var node StorageNode
	if err := collection.Find(bson.M{key: value}).One(&node); err != nil {
		return false
	} else {
		return true
	}
}

func Nodes_GET(w http.ResponseWriter, r *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	params := r.URL.Query()
	managed_state := params.Get("state")

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.AppDatabase).C(COLL_NAME_STORAGE_NODES)
	var nodes StorageNodes
	if managed_state != "" {
		if err := collection.Find(bson.M{"managedstate": managed_state}).All(&nodes); err != nil {
			glog.Errorf("Error getting the nodes list: %v", err)
		}
	} else {
		if err := collection.Find(nil).All(&nodes); err != nil {
			glog.Errorf("Error getting the nodes list: %v", err)
		}
	}

	json.NewEncoder(w).Encode(nodes)
}

func Node_GET(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	node_id := vars["node-id"]

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.AppDatabase).C(COLL_NAME_STORAGE_NODES)
	var node StorageNode
	if err := collection.Find(bson.M{"uuid": node_id}).One(&node); err != nil {
		glog.Errorf("Error getting the node detail: %v", err)
	}

	json.NewEncoder(w).Encode(node)
}
