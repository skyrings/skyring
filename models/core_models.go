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

type StorageNode struct {
	UUID              uuid.UUID          `json:"uuid"`
	Hostname          string             `json:"hostname"`
	Tags              map[string]string  `json:"tags"`
	ManagementIp      string             `json:"managementip"`
	ClusterIp         string             `json:"clusterip"`
	PublicAddressIpv4 string             `json:"publicaddressipv4"`
	ClusterId         uuid.UUID          `json:"clusterid"`
	Location          string             `json:"location"`
	Status            string             `json:"status"`
	Options           map[string]string  `json:"options"`
	State             string             `json:"state"`
	CPUs              []CPU              `json:"cpus"`
	NetworkInfo       StorageNodeNetwork `json:"networkinfo"`
	StorageDisks      []backend.Disk     `json:"storagedisks"`
	Memory            []Memory           `json:"memory"`
	OS                OperatingSystem    `json:"os"`
	ManagedState      string             `json:"managedstate"`
}

type CPU struct {
	CPUId string `bson:"cpuid"`
}

type StorageNodeNetwork struct {
	Subnet []string `bson:"subnet"`
	Ipv4   []string `bson:"ipv4"`
	Ipv6   []string `bson:"ipv6"`
}

// type StorageDisk struct {
// 	UUID       string   `bson:"uuid"`
// 	Name       string   `bson:"name"`
// 	Pkname     string   `bson:"pkname"`
// 	MountPoint []string `bson:"mountpoint"`
// 	Kname      string   `bson:"kname"`
// 	PartUUID   string   `bson:"partuuid"`
// 	Type       string   `bson:"type"`
// 	Model      string   `bson:"model"`
// 	Vendor     string   `bson:"vendor"`
// 	FsType     string   `bson:"fstype"`
// 	Size       unit64   `bson:"size"`
// 	Used       bool     `bson:"used"`
// }

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
}

type StorageCluster struct {
	ClusterId            uuid.UUID       `json:"cluster_id"`
	ClusterName          string          `json:"cluster_name"`
	CompatibilityVersion string          `json:"compatibility_version"`
	ClusterType          string          `json:"cluster_type"`
	WorkLoad             string          `json:"workload"`
	ClusterStatus        string          `json:"status"`
	Tags                 []string        `json:"tags"`
	Options              interface{}     `json:"options"`
	Nodes                []ClusterNode   `json:"nodes"`
	OpenStackServices    []string        `json:"openstackservices"`
	Networks             ClusterNetworks `json:"networks"`
}

type ClusterNode struct {
	Hostname string            `json:"hostname"`
	Options  map[string]string `json:"options"`
}

type ClusterNetworks struct {
	Cluster string `json:"cluster"`
	Public  string `json:"public"`
}
