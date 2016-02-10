// Copyright 2015 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package backend

import (
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/monitoring"
	"github.com/skyrings/skyring-common/tools/uuid"
)

type Node struct {
	Name        string
	Fingerprint string
}

type NodeList struct {
	Manage   []Node
	Unmanage []Node
	Ignore   []Node
}

type Backend interface {
	AddNode(master string, node string, port uint, fingerprint string, username string, password string, ctxt string) (bool, error)
	AcceptNode(node string, fingerprint string, ignored bool, ctxt string) (bool, error)
	BootstrapNode(master string, node string, port uint, fingerprint string, username string, password string, ctxt string) (string, error)
	GetNodes() (NodeList, error)
	GetNodeID(node string) (uuid.UUID, error)
	GetNodeDisk(node string, ctxt string) ([]models.Disk, error)
	GetNodeCpu(node string) ([]models.Cpu, error)
	GetNodeOs(node string) (models.OperatingSystem, error)
	GetNodeMemory(node string) (models.Memory, error)
	GetNodeNetwork(node string) (models.Network, error)
	IgnoreNode(node string) (bool, error)
	DisableService(node string, service string, stop bool) (bool, error)
	EnableService(node string, service string, start bool) (bool, error)
	NodeUp(node string) (bool, error)
	AddMonitoringPlugin(pluginNames []string, nodes []string, master string, pluginMap map[string]map[string]string) (failed_nodes map[string]interface{}, err error)
	UpdateMonitoringConfiguration(nodes []string, config []monitoring.Plugin) (failed_nodes map[string]string, err error)
	RemoveMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
	EnableMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
	DisableMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
}
