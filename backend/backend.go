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
	"github.com/skyrings/skyring/tools/uuid"
	"encoding/json"
	"fmt"
	"github.com/skyrings/skyring/util"
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
	DevName    string
	FSType     string
	FSUUID     uuid.UUID
	Model      string
	MountPoint []string
	Name       string
	Parent     string
	Size       uint64
	Type       string
	Used       bool
	Vendor     string
}

type Plugin struct {
	Name    string           `json:"name"`
	Enable  bool             `json:"enable"`
	Configs []PluginConfig `json:"configs"`
}

type PluginConfig struct {
	Category string `json:"category"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

var (

	SupportedConfigCategories = []string{
		"threshold",
		"interval",
		"miscellaneous",
	}

	SupportedThresholdTypes = []string{
		"critical",
		"warning",
	}
)

func (p Plugin) Valid() bool {
	validPluginName := util.Contains(p.Name, SupportedCollectdPlugins)
	if validPluginName {
		for _, config := range p.Configs {
			if !config.Valid() {
				return false
			}
		}
		return true
	}
	return false
}

func (c PluginConfig) ValidConfigType() bool {
	switch c.Category {
	case "threshold":
		return c.Type != "" && util.Contains(c.Type, SupportedThresholdTypes)
	}
	return true
}

func (c PluginConfig) ValidConfigCategory() bool {
	return util.Contains(c.Category, SupportedConfigCategories)
}

func (c PluginConfig) Valid() bool {
	return c.ValidConfigCategory() && c.ValidConfigType()
}

type collectd_config PluginConfig
type collectd_plugin Plugin

const SupportedCollectdPlugins = []string{
	"df",
	"memory",
	"cpu",
	"python",
}

func (p *Plugin) UnmarshalJSON(data []byte) (err error) {
	tPlugin := collectd_plugin{}
	if err := json.Unmarshal(data, &tPlugin); err != nil {
		return err
	}
	if !(Plugin(tPlugin)).Valid() {
		return fmt.Errorf("Couldn't Parse %v", tPlugin)
	}
	*p = Plugin(tPlugin)
	return nil
}

func (c *PluginConfig) UnmarshalJSON(data []byte) (err error) {
	tConfig := collectd_config{}
	if err := json.Unmarshal(data, &tConfig); err != nil {
		return err
	}
	if !(PluginConfig(tConfig)).Valid() {
		return fmt.Errorf("Couldn't Parse %v", tConfig)
	}
	*c = PluginConfig(tConfig)
	return nil
}

type Backend interface {
	AddNode(master string, node string, port uint, fingerprint string, username string, password string) (bool, error)
	AcceptNode(node string, fingerprint string, ignored bool) (bool, error)
	BootstrapNode(master string, node string, port uint, fingerprint string, username string, password string) (string, error)
	GetNodes() (NodeList, error)
	GetNodeID(node string) (uuid.UUID, error)
	GetNodeDisk(node string) ([]Disk, error)
	GetNodeNetwork(node string) (Network, error)
	IgnoreNode(node string) (bool, error)
	DisableService(node string, service string, stop bool) (bool, error)
	EnableService(node string, service string, start bool) (bool, error)
}
