/*Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the  specific language governing permissions and
limitations under the License.
*/
package models

import "github.com/skyrings/skyring/backend"

type AddStorageNodeRequest struct {
	Hostname       string `json:"hostname"`
	SshFingerprint string `json:"sshfingerprint"`
	User           string `json:"user"`
	Password       string `json:"password"`
	SshPort        int    `json:"sshport"`
}

type AddClusterRequest struct {
	Name                 string                     `json:"name"`
	CompatVersion        string                     `json:"compat_version"`
	Type                 string                     `json:"type"`
	WorkLoad             string                     `json:"workload"`
	Tags                 []string                   `json:"tags"`
	Options              map[string]string          `json:"options"`
	OpenStackServices    []string                   `json:"openstackservices"`
	Nodes                []ClusterNode              `json:"nodes"`
	Networks             ClusterNetworks            `json:"networks"`
	MonitoringThresholds backend.PluginThresholdMap `json:"monitoring_thresholds"`
}

type ClusterNode struct {
	NodeId   string              `json:"nodeid"`
	NodeType []string            `json:"nodetype"`
	Devices  []ClusterNodeDevice `json:"disks"`
	Options  map[string]string   `json:"options"`
}

type ClusterNodeDevice struct {
	Name    string            `json:"name"`
	FSType  string            `json:"fstype"`
	Options map[string]string `json:"options"`
}

type Nodes []Node

const (
	DEFAULT_SSH_PORT                = 22
	DEFAULT_FS_TYPE                 = "xfs"
	REQUEST_SIZE_LIMIT              = 1048576
	COLL_NAME_STORAGE_NODES         = "storage_nodes"
	COLL_NAME_STORAGE_CLUSTERS      = "storage_clusters"
	COLL_NAME_STORAGE_LOGICAL_UNITS = "storage_logical_units"
)

type Clusters []Cluster

type UnmanagedNode struct {
	Name            string `json:"name"`
	SaltFingerprint string `json:"saltfingerprint"`
}

type UnmanagedNodes []UnmanagedNode

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
	"active and available",
	"creating",
	"failed",
}

// Storage logical unit types
const (
	CEPH_OSD = 1 + iota
	CEPH_MON
)

var StorageLogicalUnitTypes = [...]string{
	"osd",
	"mon",
}

func (c ClusterStatus) String() string { return ClusterStatuses[c-1] }
