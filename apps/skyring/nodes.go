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
)

type StorageNode struct {
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

type AcceptHostRequest struct {
    Name string
    FingerPrint string
    User string
    Password string
}

type StorageNodes []StorageNode

func HostsHandler(w http.ResponseWriter, r *http.Request) {
    sessionCopy := db.GetDatastore()

    collection := sessionCopy.DB("skyring").C("hosts")
    var hosts StorageNodes
    if err := collection.Find(nil).All(&hosts); err != nil {
        glog.Errorf("Error getting the hosts list: ", err)
    }

    json.NewEncoder(w).Encode(hosts)
}

func HostHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    host_name := vars["host-name"]

    sessionCopy := db.GetDatastore()
    collection := sessionCopy.DB("skyring").C("hosts")
    var host StorageNode
    if err := collection.Find(bson.M{"name": host_name}).One(&host); err != nil {
        glog.Errorf("Error getting the host detail: ", err)
    }

    json.NewEncoder(w).Encode(host)
}

func AddHostHandler(w http.ResponseWriter, r *http.Request) {
    var host StorageNode

    body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
    if err != nil {
	   glog.Errorf("Error parsing the request: ", err)
    }
    if err := r.Body.Close(); err != nil {
	   glog.Errorf("Error parsing the request: ", err)
    }
    if err := json.Unmarshal(body, &host); err != nil {
	   w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	   w.WriteHeader(422)
       fmt.Println("Unmarshal failed")
       if err := json.NewEncoder(w).Encode(err); err != nil {
	       glog.Fatalf("Error: ", err)
	   }
       return
    }
    fmt.Println(host)
    // Add the host
    sessionCopy := db.GetDatastore()
    hosts := sessionCopy.DB("skyring").C("hosts")
    if err := hosts.Insert(host); err != nil {
        glog.Errorf("Error adding the host: ", err)
    }

    w.Header().Set("Content-Type", "application/json;charset=UTF-8")
    w.WriteHeader(http.StatusCreated)
    res,_ := json.Marshal(host)
    if err := json.NewEncoder(w).Encode(string(res)); err != nil {
	   glog.Errorf("Error: ", err)
    }
}
