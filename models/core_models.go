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

type StorageNode struct {
	UUID           string
	Hostname       string
	SshFingerprint string
	Tags           map[string]string
	MachineId      string
	ManagementIp   string
	ClusterIp      string
	PublicIp       string
	ClusterId      string
	Location       string
	Status         string
	Options        map[string]string
	State          string
	CPUs           []CPU
	NetworkInfo    StorageNodeNetwork
	StorageDisks   []StorageDisk
	Memory         []Memory
	OS             OperatingSystem
	ManagedState   string
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

type StorageNodeNetwork struct {
	Subnet []string
	Ipv4   []string
	Ipv6   []string
}

type StorageDisk struct {
	UUID       string
	Name       string
	Pkname     string
	MountPoint string
	Kname      string
	PartUUID   string
	Type       string
	Model      string
	Vendor     string
	FsType     string
	Size       int
	InUze      string
	FreeSize   int
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

const (
	DEFAULT_SSH_PORT        = 22
	REQUEST_SIZE_LIMIT      = 1048576
	COLL_NAME_STORAGE_NODES = "storage_nodes"
	NODE_STATE_FREE         = "free"
	NODE_STATE_UNMANAGED    = "unmanaged"
	NODE_STATE_USED         = "used"
)
