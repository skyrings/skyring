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
package skyring

import (
	"net/http"
)

type CoreRoute struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
	Version     int
}

var (
	CORE_ROUTES = []CoreRoute{
		{
			Name:        "GET_SshFingerprint",
			Method:      "GET",
			Pattern:     "utils/ssh_fingerprint",
			HandlerFunc: GET_SshFingerprint,
			Version:     1,
		},
		{
			Name:        "POST_Nodes",
			Method:      "POST",
			Pattern:     "nodes",
			HandlerFunc: POST_Nodes,
			Version:     1,
		},
		{
			Name:        "GET_Nodes",
			Method:      "GET",
			Pattern:     "nodes",
			HandlerFunc: GET_Nodes,
			Version:     1,
		},
		{
			Name:        "GET_Node",
			Method:      "GET",
			Pattern:     "nodes/{node-id}",
			HandlerFunc: GET_Node,
			Version:     1,
		},
		{
			Name:        "GET_Utilization",
			Method:      "GET",
			Pattern:     "nodes/{node-id}/utilization",
			HandlerFunc: GET_Utilization,
			Version:     1,
		},
		{
			Name:        "GET_UnmanagedNodes",
			Method:      "GET",
			Pattern:     "unmanaged-nodes",
			HandlerFunc: GET_UnmanagedNodes,
			Version:     1,
		},
		{
			Name:        "POST_AcceptUnamangedNode",
			Method:      "POST",
			Pattern:     "unmanaged-nodes/{hostname}/accept",
			HandlerFunc: POST_AcceptUnamangedNode,
			Version:     1,
		},
		{
			Name:        "DELETE_Node",
			Method:      "DELETE",
			Pattern:     "nodes/{node-id}",
			HandlerFunc: DELETE_Node,
			Version:     1,
		},
		{
			Name:        "DELETE_Nodes",
			Method:      "DELETE",
			Pattern:     "nodes",
			HandlerFunc: DELETE_Nodes,
			Version:     1,
		},
	}
)
