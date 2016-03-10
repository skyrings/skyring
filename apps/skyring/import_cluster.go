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
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring-common/utils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

func (a *App) ImportCluster(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	var request models.ImportClusterRequest

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request. error: %v", ctxt, err)
		HttpResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Unable to parse the request: %v",err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Unable to unmarshal request. error: %v", ctxt, err)
		HttpResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	// Check if node is valid
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := coll.Find(bson.M{"hostname": request.BootstrapNode}).One(&node); err != nil {
		logger.Get().Error(
			"%s-Invalid node %s while import cluster. error: %v",
			request.BootstrapNode,
			err)
		HttpResponse(w, http.StatusBadRequest, "Invalid bootstrap node")
		return
	}

	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for import cluster: %v", t.ID)
				// Get the specific provider and invoke the method
				provider := a.getProviderFromClusterType(request.ClusterType)
				if provider == nil {
					util.FailTask(
						fmt.Sprintf("%s-", ctxt),
						errors.New(
							fmt.Sprintf("Error getting provider for cluster type: %s",
							request.ClusterType)),
						t)
					return
				}
				err = provider.Client.Call(
					fmt.Sprintf("%s.%s", provider.Name, "ImportCluster"),
					models.RpcRequest{
						RpcRequestVars: mux.Vars(r),
						RpcRequestData: body,
						RpcRequestContext: ctxt},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK && result.Status.StatusCode != http.StatusAccepted) {
					util.FailTask(fmt.Sprintf("%s-Error importing cluster", ctxt), err, t)
					return
				}
				// Update the master task id
				providerTaskId, err = uuid.Parse(result.Data.RequestId)
				if err != nil {
					util.FailTask(
						fmt.Sprintf(
							"%s-Error parsing provider task id while importing cluster",
							ctxt),
						err,
						t)
					return
				}
				t.UpdateStatus("Started provider task: %v", *providerTaskId)
				if ok, err := t.AddSubTask(*providerTaskId); !ok || err != nil {
					util.FailTask(
						fmt.Sprintf(
							"%s-Error adding sub task while importing cluster",
							ctxt),
						err,
						t)
					return
				}

				// Check for provider task to complete
				sessionCopy := db.GetDatastore().Copy()
				defer sessionCopy.Close()
				coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
				var providerTask models.AppTask
				for {
					time.Sleep(2 * time.Second)
					if err := coll.Find(bson.M{"id": *providerTaskId}).One(&providerTask); err != nil {
						util.FailTask(
							fmt.Sprintf(
								"%s-Error getting sub task status while importing cluster",
								ctxt),
							err,
							t)
						return
					}
					if providerTask.Completed {
						if providerTask.Status == models.TASK_STATUS_SUCCESS {
							t.UpdateStatus("Success")
							t.Done(models.TASK_STATUS_SUCCESS)
						} else { //if the task is failed????
							t.UpdateStatus("Failed")
							t.Done(models.TASK_STATUS_FAILURE)
							logger.Get().Error(
								"%s- Failed to import cluster",
								ctxt)
						}
						break
					}
				}
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(
		fmt.Sprintf("Import Cluster"),
		asyncTask,
		7200*time.Second,
		nil,
		nil,
		nil); err != nil {
		logger.Get().Error(
			"%s-Unable to create task for import cluster. error: %v",
			ctxt,
			err)
		HttpResponse(
			w,
			http.StatusInternalServerError,
			"Task creation failed for import cluster")
		return
	} else {
		logger.Get().Debug(
			"%s-Task Created: %v for import cluster",
			ctxt,
			taskId)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}
