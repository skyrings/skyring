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
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/tools/gopy"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/ssh"
	"github.com/skyrings/skyring/tools/uuid"
	"strings"
	"sync"
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
	"NodeUp",
	"AddMonitoringPlugin",
	"UpdateMonitoringConfiguration",
	"EnableMonitoringPlugin",
	"DisableMonitoringPlugin",
	"RemoveMonitoringPlugin",
}

var pyFuncs map[string]*gopy.PyFunction

var mutex sync.Mutex

func init() {
	var err error
	if pyFuncs, err = gopy.Import("skyring.saltwrapper", funcNames[:]...); err != nil {
		panic(err)
	}
}

type Salt struct {
}

func (s Salt) AddNode(master string, node string, port uint, fingerprint string, username string, password string) (status bool, err error) {
	if finger, loc_err := s.BootstrapNode(master, node, port, fingerprint, username, password); loc_err == nil {
		status, err = s.AcceptNode(node, finger, false)
	} else {
		status = false
		err = loc_err
	}
	return
}

func (s Salt) AcceptNode(node string, fingerprint string, ignored bool) (status bool, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	if pyobj, loc_err := pyFuncs["AcceptNode"].Call(node, fingerprint, ignored); loc_err == nil {
		err = gopy.Convert(pyobj, &status)
	} else {
		status = false
		err = loc_err
	}
	return
}

func (s Salt) UpdateMonitoringConfiguration(nodes []string, config []monitoring.Plugin) (failed_nodes map[string]string, err error) {
	failed_nodes = make(map[string]string)
	mutex.Lock()
	defer mutex.Unlock()
	var pyobj *python.PyObject
	pyobj, err = pyFuncs["UpdateMonitoringConfiguration"].Call(nodes, monitoring.ToSaltPillarCompat(config))
	if err == nil {
		err = gopy.Convert(pyobj, &failed_nodes)
		logger.Get().Error("Update failed nodes are %v and error is %v", failed_nodes, err)
	}
	logger.Get().Error("Updating Monitoring Configuration failed with Error %v and the pyobj is %v", err, pyobj)
	return
}

func (s Salt) EnableMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error) {
	failed_nodes = make(map[string]string)
	mutex.Lock()
	defer mutex.Unlock()
	pyobj, loc_err := pyFuncs["EnableMonitoringPlugin"].Call(nodes, pluginName)
	if loc_err == nil {
		err = gopy.Convert(pyobj, &failed_nodes)
	} else {
		err = loc_err
	}
	return
}

func (s Salt) DisableMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error) {
	failed_nodes = make(map[string]string)
	mutex.Lock()
	defer mutex.Unlock()
	pyobj, loc_err := pyFuncs["DisableMonitoringPlugin"].Call(nodes, pluginName)
	if loc_err == nil {
		err = gopy.Convert(pyobj, &failed_nodes)
	} else {
		err = loc_err
	}
	return
}

func (s Salt) RemoveMonitoringPlugin(nodes []string, pluginName string) (failed_nodes map[string]string, err error) {
	failed_nodes = make(map[string]string)
	mutex.Lock()
	defer mutex.Unlock()
	pyobj, loc_err := pyFuncs["RemoveMonitoringPlugin"].Call(nodes, pluginName)
	if loc_err == nil {
		err = gopy.Convert(pyobj, &failed_nodes)
	} else {
		err = loc_err
	}
	return
}

func (s Salt) AddMonitoringPlugin(pluginNames []string, nodes []string, master string, pluginMap map[string]map[string]string) (failed_nodes map[string]interface{}, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	failed_nodes = make(map[string]interface{})
	pyobj, loc_err := pyFuncs["AddMonitoringPlugin"].Call(pluginNames, nodes, master, pluginMap)
	if loc_err == nil {
		err = gopy.Convert(pyobj, &failed_nodes)
	} else {
		err = loc_err
	}
	return
}

func (s Salt) BootstrapNode(master string, node string, port uint, fingerprint string, username string, password string) (finger string, err error) {
	var buf bytes.Buffer
	t, loc_err := template.ParseFiles("/srv/salt/template/setup-node.sh.template")
	if loc_err != nil {
		logger.Get().Critical("Error Parsing the setup-node.sh.template", err)
		return "", loc_err
	}
	if loc_err = t.Execute(&buf, struct{ Master string }{Master: master}); err != nil {
		logger.Get().Critical("Error Executing the setup-node.sh.template", err)
		return "", loc_err
	}

	if sout, _, loc_err := ssh.Run(buf.String(), node, port, fingerprint, username, password); loc_err == nil {
		return strings.TrimSpace(sout), nil
	}
	return "", loc_err
}

func (s Salt) GetNodes() (nodes backend.NodeList, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	if pyobj, loc_err := pyFuncs["GetNodes"].Call(); loc_err == nil {
		err = gopy.Convert(pyobj, &nodes)
	} else {
		err = loc_err
	}
	return
}

func (s Salt) GetNodeID(node string) (id uuid.UUID, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	pyobj, loc_err := pyFuncs["GetNodeID"].Call(node)
	if loc_err == nil {
		var s string
		loc_err = gopy.Convert(python.PyDict_GetItemString(pyobj, node), &s)
		if loc_err == nil {
			if i, loc_err := uuid.Parse(s); loc_err == nil {
				return *i, nil
			}
		}
	}
	return uuid.UUID{}, loc_err
}

func (s Salt) GetNodeDisk(node string) (disks []backend.Disk, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	if pyobj, loc_err := pyFuncs["GetNodeDisk"].Call(node); loc_err == nil {
		err = gopy.Convert(python.PyDict_GetItemString(pyobj, node), &disks)
	} else {
		err = loc_err
	}
	return
}

func (s Salt) GetNodeNetwork(node string) (n backend.Network, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	if pyobj, loc_err := pyFuncs["GetNodeNetwork"].Call(node); loc_err == nil {
		err = gopy.Convert(python.PyDict_GetItemString(pyobj, node), &n)
	} else {
		err = loc_err
	}
	return
}

func (s Salt) IgnoreNode(node string) (status bool, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	if pyobj, loc_err := pyFuncs["IgnoreNode"].Call(node); loc_err == nil {
		err = gopy.Convert(pyobj, &status)
	} else {
		err = loc_err
	}
	return
}

func (s Salt) DisableService(node string, service string, stop bool) (status bool, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	if pyobj, loc_err := pyFuncs["DisableService"].Call(node, service, stop); loc_err == nil {
		err = gopy.Convert(pyobj, &status)
	} else {
		status = false
		err = loc_err
	}
	return
}

func (s Salt) EnableService(node string, service string, start bool) (status bool, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	if pyobj, loc_err := pyFuncs["EnableService"].Call(node, service, start); loc_err == nil {
		err = gopy.Convert(pyobj, &status)
	} else {
		status = false
		err = loc_err
	}
	return
}

func (s Salt) NodeUp(node string) (status bool, err error) {
	mutex.Lock()
	defer mutex.Unlock()
	if pyobj, loc_err := pyFuncs["NodeUp"].Call(node); loc_err == nil {
		err = gopy.Convert(pyobj, &status)
	} else {
		status = false
		err = loc_err
	}
	return
}

func New() backend.Backend {
	return new(Salt)
}
