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
	"github.com/skyrings/skyring/tools/task"
	"github.com/skyrings/skyring/tools/uuid"
)

type Node struct {
	NodeId        uuid.UUID          `json:"nodeid"`
	Hostname      string             `json:"hostname"`
	Tags          []string           `json:"tags"`
	ManagementIP4 string             `json:"management_ip4"`
	ClusterIP4    string             `json:"cluster_ip4"`
	PublicIP4     string             `json:"public_ip4"`
	ClusterId     uuid.UUID          `json:"clusterid"`
	Location      string             `json:"location"`
	Status        string             `json:"status"`
	Options       map[string]string  `json:"options"`
	CPUs          []CPU              `json:"cpus"`
	NetworkInfo   StorageNodeNetwork `json:"network_info"`
	StorageDisks  []backend.Disk     `json:"storage_disks"`
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
	Name                string `bson:"name"`
	OSVersion           string `bson:"osversion"`
	KernelVersion       string `bson:"kernel_version"`
	StorageSWVersion    string `bson:"storage_sw_version"`
	KdumpStatus         string `bson:"kdump_status"`
	MemPageShareStatus  string `bson:"mem_page_share_status"`
	AutomaticLargePages bool   `bson:"automatic_large_pages"`
	SELinuxMode         string `bson:"selinuxmode"`
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
	ClusterId         uuid.UUID                `json:"clusterid"`
	Name              string                   `json:"name"`
	CompatVersion     string                   `json:"compat_version"`
	Type              string                   `json:"type"`
	WorkLoad          string                   `json:"workload"`
	Status            int                      `json:"status"`
	Tags              []string                 `json:"tags"`
	Options           map[string]string        `json:"options"`
	OpenStackServices []string                 `json:"openstack_services"`
	Networks          ClusterNetworks          `json:"networks"`
	Enabled           bool                     `json:"enabled"`
	MonitoringPlugins []backend.CollectdPlugin `json:"monitoringplugins"`
}

type ClusterNetworks struct {
	Cluster string `json:"cluster"`
	Public  string `json:"public"`
}

type StorageLogicalUnit struct {
	SluId           uuid.UUID         `json:"sluid"`
	Name            string            `json:"name"`
	Type            int               `json:"type"`
	ClusterId       uuid.UUID         `json:"clusterid"`
	NodeId          uuid.UUID         `json:"nodeid"`
	StorageId       uuid.UUID         `json:"storageid"`
	StorageDeviceId uuid.UUID         `json:"storagedeviceid"`
	Status          string            `json:"status"`
	Options         map[string]string `json:"options"`
}

type Task struct {
	Id         uuid.UUID     `json:"id"`
	Started    bool          `json:"started"`
	Completed  bool          `json:"completed"`
	StatusList []task.Status `json:"statuslist"`
}

type Storage struct {
	StorageId uuid.UUID         `json:"storageid"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Tags      []string          `json:"tags"`
	ClusterId uuid.UUID         `json:"clusterid"`
	Size      uint64            `json:"size"`
	Status    string            `json:"status"`
	Options   map[string]string `json:"options"`
}
