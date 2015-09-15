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
    "github.com/golang/glog"
    "github.com/gorilla/mux"
    "io"
    "os"
    "io/ioutil"
    "net/http"
    "skyring/db"
    "skyring/utils"
    "fmt"
    "gopkg.in/mgo.v2/bson"
    "code.google.com/p/go-uuid/uuid"
)

type StorageNode struct {
    UUID string
    Hostname string
    Tags map[string]string
    MachineId string
    ManagementIp string
    ClusterIp string
    PublicIp string
    ClusterId string
    Location string
    Status string
    Options map[string]string
    State string
    CPUs []CPU
    NetworkInterfaces []NIC
    StorageDisks []StorageDisk
    Memory []Memory
    OS OperatingSystem
    ManagedState string
}

type AddStorageNodeRequest struct {
    HostName string
    SshFingerprint string
    RootUser string
    Password string
}

type CPU struct {
    CPUId string
}

type NIC struct {
    Name string
    Type string
    Address string
    Subnet string
    Gateway string
    VLanId string
}

type StorageDisk struct {
    Name string
    Speed int
    Capacity int
    FreeSize int
    Type string
    Latency int
    Throughput int
    Location string
}

type Memory struct {
    Name string
    Type string
    TotalSize int
    FreeSize int
    Attributes string
}

type OperatingSystem struct {
    Name string
    OSVersion string
    KernelVersion string
    StorageProviderVersion string
    KdumpStatus string
    MemoryPageSharingStatus string
    AutomaticLaregPages bool
    SELinuxMode string
}

type StorageNodes []StorageNode

var (
    curr_hostname, err = os.Hostname()
)

func AddStorageNodeHandler(w http.ResponseWriter, r *http.Request) {
    var add_node_req AddStorageNodeRequest

    body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
    if err != nil {
	   glog.Errorf("Error parsing the request: ", err)
    }
    if err := r.Body.Close(); err != nil {
	   glog.Errorf("Error parsing the request: ", err)
    }
    if err := json.Unmarshal(body, &add_node_req); err != nil {
	   w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	   w.WriteHeader(422)
       fmt.Println("Unmarshal failed")
       if err := json.NewEncoder(w).Encode(err); err != nil {
	       glog.Fatalf("Error: ", err)
	   }
       return
    }

    var storage_node StorageNode

    storage_node.UUID = uuid.NewUUID().String()
    storage_node.Hostname = add_node_req.Name

    // Get the ssh-fingerprint of the node
    storage_node.SshFingerprint = add_node_req.SshFingerprint

    // Add the node
    if nodeAlreadyAdded(add_node_req.Name) {
        w.Header().Set("Content-Type", "application/json;charset=UTF-8")
        w.WriteHeader(405)
        if err := json.NewEncoder(w).Encode("Node already added"); err != nil {
            glog.Errorf("Error: ", err)
        }
        return
    }

    ret_val := util.PyAddNode(add_node_req.Name, storage_node.SshFingerprint, add_node_req.RootUser, add_node_req.Password, curr_hostname)

    if ret_val == true {
        storage_node.ManagedState = "free"
        storage_node.MachineId = util.PyGetNodeMachineId(add_node_req.Name)

        // Add the node details to the DB
        sessionCopy := db.GetDatastore()
        coll := sessionCopy.DB("skyring").C("storage_nodes")
        if err := coll.Insert(storage_node); err != nil {
            glog.Fatalf("Error adding the node: ", err)
            return
        }

        if err := json.NewEncoder(w).Encode("Added successfully"); err != nil {
            glog.Errorf("Error: ", err)
        }
    } else {
        w.Header().Set("Content-Type", "application/json;charset=UTF-8")
        w.WriteHeader(422)
        if err := json.NewEncoder(w).Encode(err); err != nil {
            glog.Errorf("Error: ", err)
        }
    }
}

func nodeAlreadyAdded(node_name string) bool {
    sessionCopy := db.GetDatastore()
    collection := sessionCopy.DB("skyring").C("storage_nodes")
    var node StorageNode
    if err := collection.Find(bson.M{"hostname": node_name}).One(&node); err != nil {
        return false
    } else {
        return true
    }
}

func StorageNodesHandler(w http.ResponseWriter, r *http.Request) {
    sessionCopy := db.GetDatastore()

    params := r.URL.Query()
    managed_state := params.Get("state")

    collection := sessionCopy.DB("skyring").C("storage_nodes")
    var nodes StorageNodes
    if managed_state != "" {
        if err := collection.Find(bson.M{"managedstate": managed_state}).All(&nodes); err != nil {
            glog.Errorf("Error getting the nodes list: ", err)
        }
    } else {
        if err := collection.Find(nil).All(&nodes); err != nil {
            glog.Errorf("Error getting the nodes list: ", err)
        }
    }

    json.NewEncoder(w).Encode(nodes)
}

func StorageNodeHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    node_id := vars["node-id"]

    sessionCopy := db.GetDatastore()
    collection := sessionCopy.DB("skyring").C("storage_nodes")
    var node StorageNode
    if err := collection.Find(bson.M{"uuid": node_id}).One(&node); err != nil {
        glog.Errorf("Error getting the node detail: ", err)
    }

    json.NewEncoder(w).Encode(node)
}

func GetNode(node_id string) StorageNode {
    sessionCopy := db.GetDatastore()
    collection := sessionCopy.DB("skyring").C("storage_nodes")
    var node StorageNode
    if err := collection.Find(bson.M{"uuid": node_id}).One(&node); err != nil {
        glog.Errorf("Error getting the node detail: ", err)
    }

    return node
}