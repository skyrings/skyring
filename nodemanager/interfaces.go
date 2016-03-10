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
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/monitoring"
)

type NodeManagerInterface interface {
	AcceptNode(node string, fingerprint string, ctxt string) (bool, error)
	AddNode(master string, node string, port uint, fingerprint string, username string, password string, ctxt string) (bool, error)
	GetUnmanagedNodes(ctxt string) (*models.UnmanagedNodes, error)
	DisableNode(node string, ctxt string) (bool, error)
	EnableNode(node string, ctxt string) (bool, error)
	SyncStorageDisks(node string, sProfiles []models.StorageProfile, ctxt string) (bool, error)
	RemoveNode(node string, ctxt string) (bool, error)
	IgnoreNode(node string, ctxt string) (bool, error)
	AddMonitoringPlugin(nodes []string, master string, plugin monitoring.Plugin, ctxt string) (failed_nodes map[string]interface{}, err error)
	RemoveMonitoringPlugin(nodes []string, pluginName string, ctxt string) (failed_nodes map[string]string, err error)
	DisableMonitoringPlugin(nodes []string, pluginName string, ctxt string) (failed_nodes map[string]string, err error)
	EnableMonitoringPlugin(nodes []string, pluginName string, ctxt string) (failed_nodes map[string]string, err error)
	UpdateMonitoringConfiguration(nodes []string, config []monitoring.Plugin, ctxt string) (failed_nodes map[string]string, err error)
	EnforceMonitoring(plugin_names []string, nodes []string, master string, plugins []monitoring.Plugin, ctxt string) (map[string]interface{}, error)
	SetUpMonitoring(node string, master string, ctxt string) (map[string]interface{}, error)
	SyncModules(node string, ctxt string) (bool, error)
	IsNodeUp(hostname string, ctxt string) (bool, error)
	GetFingerPrint(hostname string, ctxt string) (string, error)
}
