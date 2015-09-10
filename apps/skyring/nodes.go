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
    "io/ioutil"
    "net/http"
    "skyring/db"
    "gopkg.in/mgo.v2/bson"
    "fmt"
    "code.google.com/p/go-uuid/uuid"
)

type UnmanagedNode struct {
    UUID string
    Name string
    IP string
}

type ManagedNode struct {
    UUID string
    Hostname string
    Tags map[string]string
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

type AcceptNodeRequest struct {
    FingerPrint string
    User string
    Password string
}

type UnManagedNodes []UnmanagedNode

var (
    curr_hostname, err = os.Hostname()
)

func UnManagedNodesHandler(w http.ResponseWriter, r *http.Request) {
    sessionCopy := db.GetDatastore()

    collection := sessionCopy.DB("skyring").C("unmanaged_nodes")
    var nodes UnManagedNodes
    if err := collection.Find(nil).All(&nodes); err != nil {
        glog.Errorf("Error getting the nodes list: ", err)
    }

    json.NewEncoder(w).Encode(nodes)
}

func UnManagedNodeHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    node_id := vars["node-id"]

    node := GetUnManagedNode(node_id)

    json.NewEncoder(w).Encode(node)
}

func AddUnManagedNodeHandler(w http.ResponseWriter, r *http.Request) {
    var node UnmanagedNode

    body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
    if err != nil {
	   glog.Errorf("Error parsing the request: ", err)
    }
    if err := r.Body.Close(); err != nil {
	   glog.Errorf("Error parsing the request: ", err)
    }
    if err := json.Unmarshal(body, &node); err != nil {
	   w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	   w.WriteHeader(422)
       fmt.Println("Unmarshal failed")
       if err := json.NewEncoder(w).Encode(err); err != nil {
	       glog.Fatalf("Error: ", err)
	   }
       return
    }

    node.UUID = uuid.NewUUID().String()

    // Add the node
    sessionCopy := db.GetDatastore()
    nodes := sessionCopy.DB("skyring").C("unmanaged_nodes")
    if err := nodes.Insert(node); err != nil {
        glog.Errorf("Error adding the node: ", err)
    }

    w.Header().Set("Content-Type", "application/json;charset=UTF-8")
    w.WriteHeader(http.StatusCreated)
    res,_ := json.Marshal(node)
    if err := json.NewEncoder(w).Encode(string(res)); err != nil {
	   glog.Errorf("Error: ", err)
    }
}

func GetUnManagedNode(node_id string) UnmanagedNode {
    sessionCopy := db.GetDatastore()
    collection := sessionCopy.DB("skyring").C("unmanaged_nodes")
    var node UnmanagedNode
    if err := collection.Find(bson.M{"uuid": node_id}).One(&node); err != nil {
        glog.Errorf("Error getting the node detail: ", err)
    }

    return node
}
