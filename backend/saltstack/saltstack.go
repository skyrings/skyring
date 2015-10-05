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

package saltstack

import (
	"bytes"
	"github.com/skyrings/skyring/backend"
	"github.com/skyrings/skyring/ssh"
	"github.com/skyrings/skyring/tools/gopy"
	"github.com/skyrings/skyring/uuid"
	"strings"
	"text/template"
)

var functions = [...]string{
	"accept_node",
	"add_node",
	"get_nodes",
	"get_node_machine_id",
	"get_node_network_info",
	"get_node_disk_info",
}

var py_functions map[string]*gopy.PyFunction

func init() {
	var err error
	py_functions, err = gopy.Import("skyring.salt_wrapper", functions[:]...)
	if err != nil {
		panic(err)
	}
}

type salt struct {
}

func (s *salt) AddNode(master string, node string, port uint, fingerprint string, username string, password string) (status bool, err error) {
	if finger, err := s.BootstrapNode(master, node, port, fingerprint, username, password); err == nil {
		status, err = s.AcceptNode(node, finger)
	}
	return
}

func (s *salt) AcceptNode(node string, fingerprint string) (status bool, err error) {
	if pyobj, err := py_functions["accept_node"].Call(node, fingerprint); err == nil {
		err = gopy.Convert(pyobj, &status)
	}
	return
}

func (s *salt) BootstrapNode(master string, node string, port uint, fingerprint string, username string, password string) (finger string, err error) {
	var buf bytes.Buffer
	t, err := template.ParseFiles("setup-node.sh.template")
	t.Execute(&buf, struct{ Master string }{Master: master})

	if sout, _, err := ssh.Run(buf.String(), node, port, fingerprint, username, password); err == nil {
		finger = strings.TrimSpace(sout)
	}
	return
}

func (s *salt) GetNodes() (nodes map[string][]backend.Node, err error) {
	if pyobj, err := py_functions["get_nodes"].Call(); err == nil {
		err = gopy.Convert(pyobj, &nodes)
	}
	return
}

func (s *salt) GetNodeID(node string) (id uuid.UUID, err error) {
	if pyobj, err := py_functions["get_node_machine_id"].Call(node); err == nil {
		var s string
		if err = gopy.Convert(pyobj, &s); err == nil {
			if i, err := uuid.Parse(s); err == nil {
				id = *i
			}
		}
	}
	return
}

func (s *salt) GetNodeDisk(node string) (disks map[string][]backend.Disk, err error) {
	if pyobj, err := py_functions["get_node_disk_info"].Call(node); err == nil {
		err = gopy.Convert(pyobj, &disks)
	}
	return
}

func (s *salt) GetNodeNetwork(node string) (n backend.Network, err error) {
	if pyobj, err := py_functions["get_node_network_info"].Call(node); err == nil {
		err = gopy.Convert(pyobj, &n)
	}
	return
}
