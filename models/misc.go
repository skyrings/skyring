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

type AddStorageNodeRequest struct {
	Hostname       string `json:"hostname"`
	SshFingerprint string `json:"sshfingerprint"`
	User           string `json:"user"`
	Password       string `json:"password"`
	SshPort        int    `json:"sshport"`
}

type StorageNodes []StorageNode

const (
	DEFAULT_SSH_PORT           = 22
	REQUEST_SIZE_LIMIT         = 1048576
	COLL_NAME_STORAGE_NODES    = "storage_nodes"
	COLL_NAME_STORAGE_CLUSTERS = "storage_clusters"
)

type StorageClusters []StorageCluster

type UnmanagedNode struct {
	Name            string `json:"name"`
	SaltFingerprint string `json:"saltfingerprint"`
}

type UnmanagedNodes []UnmanagedNode

type AdministrativeStatus int

// Administrative status values
const (
	FREE = 1 + iota
	UNMANAGED
	USED
)

var AdminStatuses = [...]string{
	"free",
	"unmanaged",
	"used",
}

func (a AdministrativeStatus) String() string { return AdminStatuses[a-1] }

type ClusterStatus int

// Status values for the cluster
const (
	CLUSTER_STATUS_INACTIVE = 1 + iota
	CLUSTER_STATUS_NOT_AVAILABLE
	CLUSTER_STATUS_ACTIVE_AND_AVAILABLE
	CLUSTER_STATUS_CREATING
	CLUSTER_STATUS_FAILED
)

var ClusterStatuses = [...]string{
	"inactive",
	"not available",
	"active & available",
	"creating",
	"failed",
}

func (c ClusterStatus) String() string { return ClusterStatuses[c-1] }

type ExpandClusterRequest struct {
	Nodes    []ClusterNode   `json:"nodes"`
	Networks ClusterNetworks `json:"networks"`
}
