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
    "skyring/utils"
)

type SaltNodeManager struct{
    GetNodeSshFingerprint func(string) string
    AddNode func(string, string, string, string, string) bool
    GetManagedNodes func() (map[string][]string)
    GetNodeMachineId func(string) string
    GetNodeNetworkInfo func(string) map[string][]string
    GetNodeDiskInfo func(string) map[string]map[string]string
}

func NewNodeManager() *SaltNodeManager {
    manager := &SaltNodeManager{
        GetNodeSshFingerprint: util.PyGetNodeSshFingerprint,
        AddNode: util.PyAddNode,
        GetManagedNodes: util.PyGetManagedNodes,
        GetNodeMachineId: util.PyGetNodeMachineId,
        GetNodeNetworkInfo: util.PyGetNodeNetworkInfo,
        GetNodeDiskInfo: util.PyGetNodeDiskInfo,
    }

    return manager
}
