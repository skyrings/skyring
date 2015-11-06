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
	t.Execute(&buf, struct{ Master string }{Master: master})

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
