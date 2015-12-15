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
	CORE_ROUTES []CoreRoute
	//Routes that doesnot require Auth to be added here
	CORE_ROUTES_NOAUTH []CoreRoute
)

func (a *App) LoadRoutes() {
	var routes = []CoreRoute{
		{
			Name:        "GET_SshFingerprint",
			Method:      "GET",
			Pattern:     "utils/ssh_fingerprint/{hostname}",
			HandlerFunc: a.GET_SshFingerprint,
			Version:     1,
		},
		{
			Name:        "GET_LookupNode",
			Method:      "GET",
			Pattern:     "utils/lookup_node/{hostname}",
			HandlerFunc: a.GET_LookupNode,
			Version:     1,
		},
		{
			Name:        "POST_Nodes",
			Method:      "POST",
			Pattern:     "nodes",
			HandlerFunc: a.POST_Nodes,
			Version:     1,
		},
		{
			Name:        "GET_Nodes",
			Method:      "GET",
			Pattern:     "nodes",
			HandlerFunc: a.GET_Nodes,
			Version:     1,
		},
		{
			Name:        "GET_Node",
			Method:      "GET",
			Pattern:     "nodes/{node-id}",
			HandlerFunc: a.GET_Node,
			Version:     1,
		},
		{
			Name:        "DELETE_Node",
			Method:      "DELETE",
			Pattern:     "nodes/{node-id}",
			HandlerFunc: a.DELETE_Node,
			Version:     1,
		},
		{
			Name:        "DELETE_Nodes",
			Method:      "DELETE",
			Pattern:     "nodes",
			HandlerFunc: a.DELETE_Nodes,
			Version:     1,
		},
		{
			Name:        "GET_Utilization",
			Method:      "GET",
			Pattern:     "nodes/{node-id}/utilization",
			HandlerFunc: a.GET_Utilization,
			Version:     1,
		},
		{
			Name:        "GET_UnmanagedNodes",
			Method:      "GET",
			Pattern:     "unmanaged_nodes",
			HandlerFunc: a.GET_UnmanagedNodes,
			Version:     1,
		},
		{
			Name:        "POST_AcceptUnamangedNode",
			Method:      "POST",
			Pattern:     "unmanaged_nodes/{hostname}/accept",
			HandlerFunc: a.POST_AcceptUnamangedNode,
			Version:     1,
		},
		{
			Name:        "logout",
			Method:      "POST",
			Pattern:     "auth/logout",
			HandlerFunc: a.logout,
			Version:     1,
		},
		//Routes for User Management
		{
			Name:        "GET_users",
			Method:      "GET",
			Pattern:     "users",
			HandlerFunc: a.getUsers,
			Version:     1,
		},
		{
			Name:        "GET_user",
			Method:      "GET",
			Pattern:     "users/{username}",
			HandlerFunc: a.getUser,
			Version:     1,
		},
		{
			Name:        "IMPORT_users",
			Method:      "GET",
			Pattern:     "externalusers",
			HandlerFunc: a.getExternalUsers,
			Version:     1,
		},
		{
			Name:        "POST_users",
			Method:      "POST",
			Pattern:     "users",
			HandlerFunc: a.addUsers,
			Version:     1,
		},
		{
			Name:        "DELETE_users",
			Method:      "DELETE",
			Pattern:     "users/{username}",
			HandlerFunc: a.deleteUser,
			Version:     1,
		},
		{
			Name:        "PUT_users",
			Method:      "PUT",
			Pattern:     "users/{username}",
			HandlerFunc: a.modifyUsers,
			Version:     1,
		},
		//Routes for Task Management
		{
			Name:        "GET_tasks",
			Method:      "GET",
			Pattern:     "tasks",
			HandlerFunc: a.getTasks,
			Version:     1,
		},
		{
			Name:        "GET_task",
			Method:      "GET",
			Pattern:     "tasks/{taskid}",
			HandlerFunc: a.getTask,
			Version:     1,
		},
		{
			Name:        "GET_subtasks",
			Method:      "GET",
			Pattern:     "tasks/{taskid}/subtasks",
			HandlerFunc: a.getSubTasks,
			Version:     1,
		},
		//Routes for Cluster Management
		{
			Name:        "POST_Clusters",
			Method:      "POST",
			Pattern:     "clusters",
			HandlerFunc: a.POST_Clusters,
			Version:     1,
		},
		{
			Name:        "GET_Clusters",
			Method:      "GET",
			Pattern:     "clusters",
			HandlerFunc: a.GET_Clusters,
			Version:     1,
		},
		{
			Name:        "GET_Cluster",
			Method:      "GET",
			Pattern:     "clusters/{cluster-id}",
			HandlerFunc: a.GET_Cluster,
			Version:     1,
		},
		{
			Name:        "GET_MonitoringPlugins",
			Method:      "GET",
			Pattern:     "clusters/{cluster-id}/mon_plugins",
			HandlerFunc: a.GET_MonitoringPlugins,
			Version:     1,
		},
		{
			Name:        "POST_EnableMonitoringPlugin",
			Method:      "POST",
			Pattern:     "clusters/{cluster-id}/mon_plugins/{plugin-name}/enable",
			HandlerFunc: a.POST_EnableMonitoringPlugin,
			Version:     1,
		},
		{
			Name:        "POST_DisableMonitoringPlugin",
			Method:      "POST",
			Pattern:     "clusters/{cluster-id}/mon_plugins/{plugin-name}/disable",
			HandlerFunc: a.POST_DisableMonitoringPlugin,
			Version:     1,
		},
		{
			Name:        "POST_Thresholds",
			Method:      "POST",
			Pattern:     "clusters/{cluster-id}/mon_plugins/thresholds",
			HandlerFunc: a.POST_Thresholds,
			Version:     1,
		},
		{
			Name:        "REMOVE_MonitoringPlugin",
			Method:      "DELETE",
			Pattern:     "clusters/{cluster-id}/mon_plugins/{plugin-name}",
			HandlerFunc: a.REMOVE_MonitoringPlugin,
			Version:     1,
		},
		{
			Name:        "POST_AddMonitoringPlugin",
			Method:      "POST",
			Pattern:     "clusters/{cluster-id}/mon_plugins",
			HandlerFunc: a.POST_AddMonitoringPlugin,
			Version:     1,
		},
		{
			Name:        "Forget_Cluster",
			Method:      "DELETE",
			Pattern:     "clusters/{cluster-id}",
			HandlerFunc: a.Forget_Cluster,
			Version:     1,
		},
		{
			Name:        "Unmanage_Cluster",
			Method:      "POST",
			Pattern:     "clusters/{cluster-id}/unmanage",
			HandlerFunc: a.Unmanage_Cluster,
			Version:     1,
		},
		{
			Name:        "Manage_Cluster",
			Method:      "POST",
			Pattern:     "clusters/{cluster-id}/manage",
			HandlerFunc: a.Manage_Cluster,
			Version:     1,
		},
		{
			Name:        "Expand_Cluster",
			Method:      "POST",
			Pattern:     "clusters/{cluster-id}/expand",
			HandlerFunc: a.Expand_Cluster,
			Version:     1,
		},
		{
			Name:        "GET_ClusterNodes",
			Method:      "GET",
			Pattern:     "clusters/{cluster-id}/nodes",
			HandlerFunc: a.GET_ClusterNodes,
			Version:     1,
		},
		{
			Name:        "GET_ClusterNode",
			Method:      "GET",
			Pattern:     "clusters/{cluster-id}/nodes/{node-id}",
			HandlerFunc: a.GET_ClusterNode,
			Version:     1,
		},
		{
			Name:        "GET_ClusterSlus",
			Method:      "GET",
			Pattern:     "clusters/{cluster-id}/slus",
			HandlerFunc: a.GET_ClusterSlus,
			Version:     1,
		},
		{
			Name:        "GET_ClusterSlu",
			Method:      "GET",
			Pattern:     "clusters/{cluster-id}/slus/{slu-id}",
			HandlerFunc: a.GET_ClusterSlu,
			Version:     1,
		},
		{
			Name:        "POST_Storages",
			Method:      "POST",
			Pattern:     "clusters/{cluster-id}/storages",
			HandlerFunc: a.POST_Storages,
			Version:     1,
		},
		{
			Name:        "GET_Storages",
			Method:      "GET",
			Pattern:     "clusters/{cluster-id}/storages",
			HandlerFunc: a.GET_Storages,
			Version:     1,
		},
		{
			Name:        "GET_Storage",
			Method:      "GET",
			Pattern:     "clusters/{cluster-id}/storages/{storage-id}",
			HandlerFunc: a.GET_Storage,
			Version:     1,
		},
		{
			Name:        "GET_AllStorages",
			Method:      "GET",
			Pattern:     "storages",
			HandlerFunc: a.GET_AllStorages,
			Version:     1,
		},
		{
			Name:        "GET_events",
			Method:      "GET",
			Pattern:     "events",
			HandlerFunc: GetEvents,
			Version:     1,
		},
	}
	for _, route := range routes {
		CORE_ROUTES = append(CORE_ROUTES, route)
	}

	var noauth_routes = []CoreRoute{
		{
			Name:        "login",
			Method:      "POST",
			Pattern:     "auth/login",
			HandlerFunc: a.login,
			Version:     1,
		},
	}
	for _, route := range noauth_routes {
		CORE_ROUTES_NOAUTH = append(CORE_ROUTES_NOAUTH, route)
	}
}
