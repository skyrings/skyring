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
package saltnodemanager

import (
	"github.com/skyrings/skyring/nodemanager"
	"github.com/skyrings/skyring/utils"
	"io"
)

const (
	NodeManagerName = "SaltNodeManager"
)

type SaltNodeManager struct {
}

func init() {
	nodemanager.RegisterNodeManager(NodeManagerName, func(config io.Reader) (nodemanager.NodeManagerInterface, error) {
		return NewSaltNodeManager(config)
	})
}

func NewSaltNodeManager(config io.Reader) (*SaltNodeManager, error) {
	return &SaltNodeManager{}, nil
}

func (a SaltNodeManager) GetNodeSshFingerprint(node string) string {
	return util.PyGetNodeSshFingerprint(node)
}

func (a SaltNodeManager) AcceptNode(node string, fingerprint string) bool {
	return util.PyAcceptNode(node, fingerprint)
}

func (a SaltNodeManager) AddNode(node string, fingerprint string, username string, password string, master string, port int) bool {
	return util.PyAddNode(node, fingerprint, username, password, master, port)
}

func (a SaltNodeManager) GetNodes() map[string]map[string]string {
	return util.PyGetNodes()
}

func (a SaltNodeManager) GetNodeMachineId(node string) string {
	return util.PyGetNodeMachineId(node)
}

func (a SaltNodeManager) GetNodeNetworkInfo(node string) map[string][]string {
	return util.PyGetNodeNetworkInfo(node)
}

func (a SaltNodeManager) GetNodeDiskInfo(node string) map[string]map[string]string {
	return util.PyGetNodeDiskInfo(node)
}

func (a SaltNodeManager) RemoveNode(node string) bool {
	return util.PyRemoveNode(node)
}
