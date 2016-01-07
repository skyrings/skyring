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
package app

import (
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/conf"
	"net/http"
)

type Application interface {
	SetRoutes(container *mux.Router) error
	InitializeNodeManager(config conf.NodeManagerConfig) error
	InitializeMonitoringManager(config conf.MonitoringDBconfig)
	//Initialize the auth module
	InitializeAuth(authCfg conf.AuthConfig) error
	//Middleware to check the request is authenticated
	LoginRequired(w http.ResponseWriter, r *http.Request, next http.HandlerFunc)
	InitializeTaskManager() error
	SyncClusterDetails()
	InitializeDb(authCfg conf.AppDBConfig) error
	InitializeDefaults() error
}
