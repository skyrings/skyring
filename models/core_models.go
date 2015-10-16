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
package models

import (
	"github.com/skyrings/skyring/backend"
	"github.com/skyrings/skyring/tools/uuid"
)

type Node struct {
	NodeId        uuid.UUID          `json:"nodeid"`
	Hostname      string             `json:"hostname"`
	Tags          []string           `json:"tags"`
	ManagementIP4 string             `json:"managementip4"`
	ClusterIP4    string             `json:"clusterip4"`
	PublicIP4     string             `json:"publicip4"`
	ClusterId     uuid.UUID          `json:"clusterid"`
	Location      string             `json:"location"`
	Status        string             `json:"status"`
	Options       map[string]string  `json:"options"`
	CPUs          []CPU              `json:"cpus"`
	NetworkInfo   StorageNodeNetwork `json:"networkinfo"`
	StorageDisks  []backend.Disk     `json:"storagedisks"`
	Memory        []Memory           `json:"memory"`
	OS            OperatingSystem    `json:"os"`
	Enabled       bool               `json:"enabled"`
}

type CPU struct {
	CPUId string `bson:"cpuid"`
}

type StorageNodeNetwork struct {
	Subnet []string `bson:"subnet"`
	Ipv4   []string `bson:"ipv4"`
	Ipv6   []string `bson:"ipv6"`
}

type Memory struct {
	Name       string `bson:"name"`
	Type       string `bson:"type"`
	TotalSize  int    `bson:"totalsize"`
	FreeSize   int    `bson:"freesize"`
	Attributes string `bson:"attribute"`
}

type OperatingSystem struct {
	Name                    string `bson:"name"`
	OSVersion               string `bson:"osversion"`
	KernelVersion           string `bson:"kernelversion"`
	StorageProviderVersion  string `bson:"storageproviderversion"`
	KdumpStatus             string `bson:"kdumpstatus"`
	MemoryPageSharingStatus string `bson:"memorypagesharingstatus"`
	AutomaticLargePages     bool   `bson:"automaticlargepages"`
	SELinuxMode             string `bson:"selinuxmode"`
}

type User struct {
	Username string   `bson:"Username"`
	Email    string   `bson:"Email"`
	Hash     []byte   `bson:"Hash"`
	Role     string   `bson:"Role"`
	Groups   []string `bson:"Groups"`
	Type     int      `bson:"Type"`
	Status   bool     `bson:"Status"`
}

type Cluster struct {
	ClusterId            uuid.UUID         `json:"clusterid"`
	ClusterName          string            `json:"clustername"`
	CompatibilityVersion string            `json:"compatibilityversion"`
	ClusterType          string            `json:"clustertype"`
	WorkLoad             string            `json:"workload"`
	ClusterStatus        int               `json:"status"`
	Tags                 []string          `json:"tags"`
	Options              map[string]string `json:"options"`
	OpenStackServices    []string          `json:"openstackservices"`
	Networks             ClusterNetworks   `json:"networks"`
	Enabled              bool              `json:"enabled"`
}

type ClusterNetworks struct {
	Cluster string `json:"cluster"`
	Public  string `json:"public"`
}

type StorageLogicalUnit struct {
	SluId           uuid.UUID         `json:"sluid"`
	SluName         string            `json:"sluname"`
	Type            int               `json:"type"`
	ClusterId       uuid.UUID         `json:"clusterid"`
	NodeId          uuid.UUID         `json:"nodeid"`
	StorageId       uuid.UUID         `json:"storageid"`
	StorageDeviceId uuid.UUID         `json:"storagedeviceid"`
	Status          string            `json:"status"`
	Options         map[string]string `json:"options"`
}
