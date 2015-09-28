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
	"github.com/skyrings/skyring/uuid"
)

type StorageNode struct {
	UUID              uuid.UUID          `bson:"uuid"`
	Hostname          string             `bson:"hostname"`
	Tags              map[string]string  `bson:"tags"`
	ManagementIp      string             `bson:"managementip"`
	ClusterIp         string             `bson:"clusterip"`
	PublicAddressIpv4 string             `bson:"publicaddressipv4"`
	ClusterId         string             `bson:"clusterid"`
	Location          string             `bson:"location"`
	Status            string             `bson:"status"`
	Options           map[string]string  `bson:"options"`
	State             string             `bson:"state"`
	CPUs              []CPU              `bson:"cpus"`
	NetworkInfo       StorageNodeNetwork `bson:"networkinfo"`
	StorageDisks      []StorageDisk      `bson:"storagedisks"`
	Memory            []Memory           `bson:"memory"`
	OS                OperatingSystem    `bson:"os"`
	ManagedState      string             `bson:"managedstate"`
}

type AddStorageNodeRequest struct {
	Hostname       string `json:"hostname"`
	SshFingerprint string `json:"sshfingerprint"`
	User           string `json:"user"`
	Password       string `json:"password"`
	SshPort        int    `json:"sshport"`
}

type CPU struct {
	CPUId string `bson:"cpuid"`
}

type StorageNodeNetwork struct {
	Subnet []string `bson:"subnet"`
	Ipv4   []string `bson:"ipv4"`
	Ipv6   []string `bson:"ipv6"`
}

type StorageDisk struct {
	UUID       string `bson:"uuid"`
	Name       string `bson:"name"`
	Pkname     string `bson:"pkname"`
	MountPoint string `bson:"mountpoint"`
	Kname      string `bson:"kname"`
	PartUUID   string `bson:"partuuid"`
	Type       string `bson:"type"`
	Model      string `bson:"model"`
	Vendor     string `bson:"vendor"`
	FsType     string `bson:"fstype"`
	Size       int    `bson:"size"`
	InUze      string `bson:"inuze"`
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

type StorageNodes []StorageNode

type UnmanagedNode struct {
	Name            string `json:"name"`
	SaltFingerprint string `json:"saltfingerprint"`
}

const (
	DEFAULT_SSH_PORT        = 22
	REQUEST_SIZE_LIMIT      = 1048576
	COLL_NAME_STORAGE_NODES = "storage_nodes"
	NODE_STATE_FREE         = "free"
	NODE_STATE_UNMANAGED    = "unmanaged"
	NODE_STATE_USED         = "used"
)

type User struct {
	Username string   `bson:"Username"`
	Email    string   `bson:"Email"`
	Hash     []byte   `bson:"Hash"`
	Role     string   `bson:"Role"`
	Groups   []string `bson:"Groups"`
}
