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
	"encoding/json"
	"fmt"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"io"
	"io/ioutil"
	"net/http"
)

func (a *App) DiskHierarchy(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	// Unmarshal the request body
	var request models.DiskHierarchyRequest
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["GET_DISK_HIERARCHY"],
			"Failed to get disk hierarchy",
			fmt.Sprintf("Failed to get disk hierarchy. error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log get disk hierarchy event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the request. error: %v", err))
		return
	}

	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["GET_DISK_HIERARCHY"],
			"Failed to get disk hierarchy",
			fmt.Sprintf("Failed to get disk hierarchy. Error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log disk hierarhcy event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	provider := a.getProviderFromClusterType(request.ClusterType)
	if provider == nil {
		if err := logAuditEvent(EventTypes["GET_DISK_HIERARCHY"],
			"Failed to get disk hierarchy",
			fmt.Sprintf("Failed to get disk hierarchy. Error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log disk hierarhcy event. Error: %v", ctxt, err)
		}
		HttpResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to get provider for cluster type: %s", request.ClusterType))
		return
	}

	var result models.RpcResponse
	err = provider.Client.Call(
		fmt.Sprintf("%s.GetDiskHierarchy", request.ClusterType),
		models.RpcRequest{RpcRequestVars: map[string]string{}, RpcRequestData: body, RpcRequestContext: ctxt},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		if err := logAuditEvent(EventTypes["GET_DISK_HIERARCHY"],
			"Failed to get disk hierarchy",
			fmt.Sprintf("Failed to get disk hierarchy. Error: %v", fmt.Errorf("Provider task failed")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_CLUSTER,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log disk hierarhcy event. Error: %v", ctxt, err)
		}
		HttpResponse(
			w,
			http.StatusInternalServerError,
			fmt.Sprintf("Failed to get disk hierarchy details. error: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(result.Data.Result)
}
