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
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/tools/uuid"
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

type Network struct {
	IPv4   []string `bson:"ipv4",json:"ipv4"`     // TODO: use ipv4 type
	IPv6   []string `bson:"ipv6",json:"ipv6"`     // TODO: use ipv6 type
	Subnet []string `bson:"subnet",json:"subnet"` // TODO: use subnet type
}

type Disk struct {
	DevName        string    `bson:"devname",json:"devname"`
	FSType         string    `bson:"fstype",json:"fstype"`
	FSUUID         uuid.UUID `bson:"fsuuid",json:"fsuuid"`
	Model          string    `bson:"model",json:"model"`
	MountPoint     []string  `bson:"mountpoint",json:"mountpoint"`
	Name           string    `bson:"name",json:"name"`
	Parent         string    `bson:"parent",json:"parent"`
	Size           uint64    `bson:"size",json:"size"`
	Type           string    `bson:"type",json:"type"`
	Used           bool      `bson:"used",json:"used"`
	SSD            bool      `bson:"ssd",json:"ssd"`
	Vendor         string    `bson:"vendor",json:"vendor"`
	StorageProfile string    `bson:"storageprofile",json:"storageprofile"`
	DiskId         uuid.UUID `bson:"diskid",json:"diskid"`
}

type Cpu struct {
	Architecture   string `bson:"architecture",json:"architecture"`
	CpuOpMode      string `bson:"cpuopmode",json:"cpuopmode"`
	CPUs           string `bson:"cpus",json:"cpus"`
	VendorId       string `bson:"vendorid",json:"vendorid"`
	ModelName      string `bson:"modelname",json:"modelname"`
	CPUFamily      string `bson:"cpufamily",json:"cpufamily"`
	CPUMHz         string `bson:"cpumhz",json:"cpumhz"`
	Model          string `bson:"model",json:"model"`
	CoresPerSocket string `bson:"corespersocket",json:"corespersocket"`
}

type OperatingSystem struct {
	Name          string `bson:"name",json:"name"`
	OSVersion     string `bson:"osversion",json:"osversion"`
	KernelVersion string `bson:"kernelversion",json:"kernelversion"`
	SELinuxMode   string `bson:"selinuxmode",json:"selinuxmode"`
}

type Memory struct {
	TotalSize string `bson:"totalsize",json:"totalsize"`
	SwapTotal string `bson:"swaptotal",json:"swaptotal"`
	Active    string `bson:"active",json:"active"`
	Type      string `bson:"type",json:"type"`
}

type Backend interface {
	AddNode(master string, node string, port uint, fingerprint string, username string, password string) (bool, error)
	AcceptNode(node string, fingerprint string, ignored bool) (bool, error)
	BootstrapNode(master string, node string, port uint, fingerprint string, username string, password string) (string, error)
	GetNodes() (NodeList, error)
	GetNodeID(node string) (uuid.UUID, error)
	GetNodeDisk(node string) ([]Disk, error)
	GetNodeCpu(node string) ([]Cpu, error)
	GetNodeOs(node string) (OperatingSystem, error)
	GetNodeMemory(node string) (Memory, error)
	GetNodeNetwork(node string) (Network, error)
	IgnoreNode(node string) (bool, error)
	DisableService(node string, service string, stop bool) (bool, error)
	EnableService(node string, service string, start bool) (bool, error)
	NodeUp(node string) (bool, error)
	AddMonitoringPlugin(pluginNames []string, nodes []string, master string, pluginMap map[string]map[string]string) (failed_nodes map[string]interface{}, err error)
	UpdateMonitoringConfiguration(nodes []string, config []monitoring.Plugin) (failed_nodes []string, err error)
	RemoveMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
	EnableMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
	DisableMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error)
}
