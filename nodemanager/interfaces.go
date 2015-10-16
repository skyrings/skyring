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
package nodemanager

import (
	"github.com/skyrings/skyring/models"
)

type NodeManagerInterface interface {
	AcceptNode(node string, fingerprint string) (*models.Node, error)
	AddNode(master string, node string, port uint, fingerprint string, username string, password string) (*models.Node, error)
	GetUnmanagedNodes() (*models.UnmanagedNodes, error)
	DisableNode(node string) (bool, error)
	EnableNode(node string) (bool, error)
	SyncStorageDisks(node string) (bool, error)
}
