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
	IPv4   []string // TODO: use ipv4 type
	IPv6   []string // TODO: use ipv6 type
	Subnet []string // TODO: use subnet type
}

type Disk struct {
	DevName        string
	FSType         string
	FSUUID         uuid.UUID
	Model          string
	MountPoint     []string
	Name           string
	Parent         string
	Size           uint64
	Type           string
	Used           bool
	Vendor         string
	StorageProfile string
}

type Cpu struct {
	CPUs                string
	Byte_Order          string
	L1d_cache           string
	CPU_op_modes        string
	CPU_MHz             string
	Model_name          string
	NUMA_nodes          string
	Vendor_ID           string
	On_line_CPUs_list   string
	CPU_family          string
	Cores_per_socket    string
	L1i_cache           string
	NUMA_node0_CPUs     string
	L2_cache            string
	Architecture        string
	Model               string
	Virtualization_type string
	Sockets             string
	Hypervisor_vendor   string
	BogoMIPS            string
	Stepping            string
	Threads_per_core    string
}

type OperationSystem struct {
	Release        string
	Distributor_ID string
	LSB_Version    string
}

type Memory struct {
	WritebackTmp      string
	SwapTotal         string
	Activeanon        string
	SwapFree          string
	DirectMap4k       string
	KernelStack       string
	MemFree           string
	HugePages_Rsvd    string
	Committed_AS      string
	Activefile        string
	NFS_Unstable      string
	VmallocChunk      string
	Writeback         string
	Inactivefile      string
	MemTotal          string
	VmallocUsed       string
	HugePages_Free    string
	AnonHugePages     string
	AnonPages         string
	Active            string
	Inactiveanon      string
	CommitLimit       string
	Hugepagesize      string
	Cached            string
	SwapCached        string
	VmallocTotal      string
	Shmem             string
	Mapped            string
	SUnreclaim        string
	Unevictable       string
	SReclaimable      string
	MemAvailable      string
	Mlocked           string
	DirectMap2M       string
	HugePages_Surp    string
	Bounce            string
	Inactive          string
	PageTables        string
	HardwareCorrupted string
	HugePages_Total   string
	Slab              string
	Buffers           string
	Dirty             string
}

type Backend interface {
	AddNode(master string, node string, port uint, fingerprint string, username string, password string) (bool, error)
	AcceptNode(node string, fingerprint string, ignored bool) (bool, error)
	BootstrapNode(master string, node string, port uint, fingerprint string, username string, password string) (string, error)
	GetNodes() (NodeList, error)
	GetNodeID(node string) (uuid.UUID, error)
	GetNodeDisk(node string) ([]Disk, error)
	GetNodeCpu(node string) (Cpu, error)
	GetNodeOs(node string) (OperationSystem, error)
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
