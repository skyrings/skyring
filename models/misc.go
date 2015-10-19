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
	"github.com/skyrings/skyring/tools/uuid"
	"time"
)

type AddStorageNodeRequest struct {
	Hostname       string `json:"hostname"`
	SshFingerprint string `json:"sshfingerprint"`
	User           string `json:"user"`
	Password       string `json:"password"`
	SshPort        int    `json:"sshport"`
}

type NodeEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	Node      string            `json:"node"`
	Tag       string            `json:"tag"`
	Tags      map[string]string `json:"tags"`
	Message   string            `json:"message"`
	Severity  string            `json:"severity"`
}

type NodeEventDoc struct {
	EventId   uuid.UUID         `json:"event_id"`
	ClusterId uuid.UUID         `json:"cluster_id"`
	NodeId    uuid.UUID         `json:"node_id"`
	Timestamp time.Time         `json:"timestamp"`
	Tag       string            `json:"tag"`
	Tags      map[string]string `json:"tags"`
	Message   string            `json:"message"`
	Severity  string            `json:"severity"`
}

type StorageNodes []StorageNode

const (
	DEFAULT_SSH_PORT           = 22
	REQUEST_SIZE_LIMIT         = 1048576
	COLL_NAME_STORAGE_NODES    = "storage_nodes"
	COLL_NAME_STORAGE_CLUSTERS = "storage_clusters"
	COLL_NAME_NODE_EVENTS      = "node_events"
	NODE_STATE_FREE            = "free"
	NODE_STATE_UNMANAGED       = "unmanaged"
	NODE_STATE_USED            = "used"
)

type StorageClusters []StorageCluster

type UnmanagedNode struct {
	Name            string `json:"name"`
	SaltFingerprint string `json:"saltfingerprint"`
}

type UnmanagedNodes []UnmanagedNode

type AsyncResponse struct {
	TaskId string
}
