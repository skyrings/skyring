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

package salt

import (
	"bytes"
	"github.com/sbinet/go-python"
	"github.com/skyrings/skyring/backend"
	"github.com/skyrings/skyring/tools/gopy"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/ssh"
	"github.com/skyrings/skyring/tools/uuid"
	"strings"
	"text/template"
)

var funcNames = [...]string{
	"AcceptNode",
	"GetNodes",
	"GetNodeID",
	"GetNodeNetwork",
	"GetNodeDisk",
	"IgnoreNode",
	"DisableService",
	"EnableService",
	"ConfigureCollectdPhysicalResources",
	"UpdateCollectdThresholds",
	"EnableCollectdPlugin",
	"DisableCollectdPlugin",
	"RemoveCollectdPlugin",
}

var pyFuncs map[string]*gopy.PyFunction

func init() {
	var err error
	if pyFuncs, err = gopy.Import("skyring.saltwrapper", funcNames[:]...); err != nil {
		panic(err)
	}
}

type Salt struct {
}

func (s Salt) AddNode(master string, node string, port uint, fingerprint string, username string, password string) (status bool, err error) {
	if finger, err := s.BootstrapNode(master, node, port, fingerprint, username, password); err == nil {
		status, err = s.AcceptNode(node, finger, false)
	}
	return
}

func (s Salt) AcceptNode(node string, fingerprint string, ignored bool) (status bool, err error) {
	if pyobj, err := pyFuncs["AcceptNode"].Call(node, fingerprint, ignored); err == nil {
		err = gopy.Convert(pyobj, &status)
	}
	return
}

func (s Salt) BootstrapNode(master string, node string, port uint, fingerprint string, username string, password string) (finger string, err error) {
	var buf bytes.Buffer
	t, err := template.ParseFiles("setup-node.sh.template")
	if err != nil {
		logger.Get().Critical("Error Parsing the setup-node.sh.template", err)
		return
	}
	if err = t.Execute(&buf, struct{ Master string }{Master: master}); err != nil {
		logger.Get().Critical("Error Executing the setup-node.sh.template", err)
		return
	}

	if sout, _, err := ssh.Run(buf.String(), node, port, fingerprint, username, password); err == nil {
		finger = strings.TrimSpace(sout)
	}
	return
}

func (s Salt) GetNodes() (nodes backend.NodeList, err error) {
	if pyobj, err := pyFuncs["GetNodes"].Call(); err == nil {
		err = gopy.Convert(pyobj, &nodes)
	}
	return
}

func (s Salt) GetNodeID(node string) (id uuid.UUID, err error) {
	if pyobj, err := pyFuncs["GetNodeID"].Call(node); err == nil {
		var s string
		if err = gopy.Convert(python.PyDict_GetItemString(pyobj, node), &s); err == nil {
			if i, err := uuid.Parse(s); err == nil {
				id = *i
			}
		}
	}
	return
}

func (s Salt) GetNodeDisk(node string) (disks []backend.Disk, err error) {
	if pyobj, err := pyFuncs["GetNodeDisk"].Call(node); err == nil {
		err = gopy.Convert(python.PyDict_GetItemString(pyobj, node), &disks)
	}
	return
}

func (s Salt) GetNodeNetwork(node string) (n backend.Network, err error) {
	if pyobj, err := pyFuncs["GetNodeNetwork"].Call(node); err == nil {
		err = gopy.Convert(python.PyDict_GetItemString(pyobj, node), &n)
	}
	return
}

func (s Salt) ConfigureCollectdPhysicalResources(node string, master string) (success bool, err error) {
	var nodes []string
	nodes = append(nodes, node)
	pyobj, err := pyFuncs["ConfigureCollectdPhysicalResources"].Call(backend.SupportedCollectdPlugins, nodes, master, backend.ToSaltPillarCompat(backend.GetDefaultThresholdValues()))
	if err != nil {
		err = gopy.Convert(pyobj, &success)
	}
	return
}

func (s Salt) UpdateCollectdThresholds(nodes []string, threshold []backend.CollectdPlugin) (err error) {
	if _, err := pyFuncs["UpdateCollectdThresholds"].Call(nodes, backend.ToSaltPillarCompat(threshold)); err != nil {
		//err = gopy.Convert(pyobj, &success)
	}
	return
}

func (s Salt) EnableCollectdPlugin(nodes []string, pluginName string) (success bool, err error) {
	pyobj, err := pyFuncs["EnableCollectdPlugin"].Call(nodes, pluginName)
	if err != nil {
		err = gopy.Convert(pyobj, &success)
	}
	return
}

func (s Salt) RemoveCollectdPlugin(nodes []string, pluginName string) (success bool, err error) {
	pyobj, err := pyFuncs["RemoveCollectdPlugin"].Call(nodes, pluginName)
	if err != nil {
		err = gopy.Convert(pyobj, &success)
	}
	return
}

func (s Salt) AddMonitoringPlugin(nodes []string, plugin backend.CollectdPlugin) (success bool, err error) {
	var pluginNames []string
	pluginNames = append(pluginNames, plugin.Name)
	var plugins []backend.CollectdPlugin
	plugins = append(plugins, plugin)
	pyobj, err := pyFuncs["ConfigureCollectdPhysicalResources"].Call(pluginNames, nodes, "", backend.ToSaltPillarCompat(plugins))
	if err != nil {
		err = gopy.Convert(pyobj, &success)
	}
	return
}

func (s Salt) DisableCollectdPlugin(nodes []string, pluginName string) (success bool, err error) {
	pyobj, err := pyFuncs["DisableCollectdPlugin"].Call(nodes, pluginName)
	if err != nil {
		err = gopy.Convert(pyobj, &success)
	}
	return
}

func (s Salt) IgnoreNode(node string) (status bool, err error) {
	if pyobj, err := pyFuncs["IgnoreNode"].Call(node); err == nil {
		err = gopy.Convert(pyobj, &status)
	}
	return
}

func (s Salt) DisableService(node string, service string, stop bool) (status bool, err error) {
	if pyobj, err := pyFuncs["DisableService"].Call(node, service, stop); err == nil {
		err = gopy.Convert(pyobj, &status)
	}
	return
}

func (s Salt) EnableService(node string, service string, start bool) (status bool, error error) {
	if pyobj, err := pyFuncs["EnableService"].Call(node, service, start); err == nil {
		err = gopy.Convert(pyobj, &status)
	}
	return
}

func New() backend.Backend {
	return new(Salt)
}
