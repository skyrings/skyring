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
	"github.com/skyrings/skyring/monitoring"
)

type NodeManagerInterface interface {
	AcceptNode(node string, fingerprint string) (bool, error)
	AddNode(master string, node string, port uint, fingerprint string, username string, password string) (bool, error)
	GetUnmanagedNodes() (*models.UnmanagedNodes, error)
	DisableNode(node string) (bool, error)
	EnableNode(node string) (bool, error)
	SyncStorageDisks(node string, sProfiles []models.StorageProfile) (bool, error)
	RemoveNode(node string) (bool, error)
	IgnoreNode(node string) (bool, error)
	AddMonitoringPlugin(nodes []string, master string, plugin monitoring.Plugin) (failed_nodes map[string]interface{}, err error)
	RemoveMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
	DisableMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
	EnableMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
	UpdateMonitoringConfiguration(nodes []string, config []monitoring.Plugin) (failed_nodes []string, err error)
	SetUpMonitoring(node string, master string) (map[string]interface{}, error)
}
