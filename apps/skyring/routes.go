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
	//Routes that require Auth to be added here
	CORE_ROUTES = []CoreRoute{
		{
			Name:        "GET_SshFingerprint",
			Method:      "GET",
			Pattern:     "utils/ssh_fingerprint/{hostname}",
			HandlerFunc: GET_SshFingerprint,
			Version:     1,
		},
		{
			Name:        "GET_LookupNode",
			Method:      "GET",
			Pattern:     "utils/lookup_node/{hostname}",
			HandlerFunc: GET_LookupNode,
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
			Pattern:     "unmanaged_nodes",
			HandlerFunc: GET_UnmanagedNodes,
			Version:     1,
		},
		{
			Name:        "POST_AcceptUnamangedNode",
			Method:      "POST",
			Pattern:     "unmanaged_nodes/{hostname}/accept",
			HandlerFunc: POST_AcceptUnamangedNode,
			Version:     1,
		},
		{
			Name:        "logout",
			Method:      "POST",
			Pattern:     "auth/logout",
			HandlerFunc: logout,
			Version:     1,
		},
		{
			Name:        "GET_users",
			Method:      "GET",
			Pattern:     "users",
			HandlerFunc: getUsers,
			Version:     1,
		},
		{
			Name:        "GET_user",
			Method:      "GET",
			Pattern:     "users/{username}",
			HandlerFunc: getUser,
			Version:     1,
		},
		{
			Name:        "IMPORT_users",
			Method:      "GET",
			Pattern:     "externalusers",
			HandlerFunc: getExternalUsers,
			Version:     1,
		},
		{
			Name:        "POST_users",
			Method:      "POST",
			Pattern:     "users",
			HandlerFunc: addUsers,
			Version:     1,
		},
		{
			Name:        "DELETE_users",
			Method:      "DELETE",
			Pattern:     "users/{username}",
			HandlerFunc: deleteUser,
			Version:     1,
		},
	}
	//Routes that doesnot require Auth to be added here
	CORE_ROUTES_NOAUTH = []CoreRoute{
		{
			Name:        "login",
			Method:      "POST",
			Pattern:     "auth/login",
			HandlerFunc: login,
			Version:     1,
		},
	}
)
