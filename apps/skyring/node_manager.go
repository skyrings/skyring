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

type NodeManager interface {
    GetNodeSshFingerprint(node string) string
    AddNode(node string, fingerprint string, username string, password string, master string) bool
    GetManagedNodes() map[string][]string
    GetNodeMachineId(node string) string
    GetNodeNetworkInfo(node string) map[string][]string
    GetNodeDiskInfo(node string) map[string]map[string]string
}
