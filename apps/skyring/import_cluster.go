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
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/monitoring"
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

func (a *App) GET_ClusterNodesForImport(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	vars := mux.Vars(r)
	params := r.URL.Query()
	bootstrapNode := params.Get("bootstrapnode")
	clusterType := params.Get("clustertype")
	vars["bootstrapnode"] = bootstrapNode

	if bootstrapNode == "" || clusterType == "" {
		logger.Get().Error(
			"%s-Required details not provided. error: %v",
			ctxt,
			err)
		HttpResponse(w, http.StatusBadRequest, "Required details not provided")
		return
	}

	ok, err := checkAndAcceptNode(bootstrapNode, ctxt)
	if err != nil || !ok {
		logger.Get().Error(
			"%s-Error accepting the bootstarp node: %s. error: %v",
			ctxt,
			bootstrapNode,
			err)
		HttpResponse(w, http.StatusInternalServerError, "Error accepting bootstrap node")
		return
	}

	// Get the specific provider
	provider := a.getProviderFromClusterType(clusterType)
	if provider == nil {
		logger.Get().Error(
			"%s-Error getting provider for cluster type: %s",
			ctxt,
			clusterType)
		HttpResponse(
			w,
			http.StatusInternalServerError,
			"Error getting provider for cluster type")
		return
	}
	var result models.RpcResponse
	err = provider.Client.Call(
		fmt.Sprintf("%s.%s", provider.Name, "GetClusterNodesForImport"),
		models.RpcRequest{
			RpcRequestVars:    vars,
			RpcRequestData:    []byte{},
			RpcRequestContext: ctxt},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error(
			"%s-Error getting import cluster details",
			ctxt)
		HttpResponse(
			w,
			http.StatusInternalServerError,
			"Error getting import cluster details")
		return
	} else {
		var cluster models.ClusterForImport
		if err := json.Unmarshal(result.Data.Result, &cluster); err != nil {
			logger.Get().Error(
				"%s-Error un-marshalling import cluster details",
				ctxt)
			HttpResponse(
				w,
				http.StatusInternalServerError,
				"Error un-marshalling import cluster details")
			return
		}
		json.NewEncoder(w).Encode(cluster)
	}
}

func (a *App) ImportCluster(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	var request models.ImportClusterRequest
	vars := mux.Vars(r)

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error(
			"%s-Error parsing the request. error: %v",
			ctxt,
			err)
		HttpResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error(
			"%s-Unable to unmarshal request. error: %v",
			ctxt,
			err)
		HttpResponse(
			w,
			http.StatusBadRequest,
			fmt.Sprintf("Unable to unmarshal request. error: %v", err))
		return
	}

	if request.BootstrapNode == "" ||
		request.ClusterType == "" ||
		len(request.Nodes) == 0 {
		logger.Get().Error(
			"%s-Required details not provided. error: %v",
			ctxt,
			err)
		HttpResponse(w, http.StatusBadRequest, "Required details not provided")
		return
	}

	var result models.RpcResponse
	var providerTaskId *uuid.UUID
	asyncTask := func(t *task.Task) {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		for {
			select {
			case <-t.StopCh:
				return
			default:
				t.UpdateStatus("Started the task for import cluster: %v", t.ID)
				t.UpdateStatus("Checking/accepting participating nodes")
				var acceptFailedNodes []string
				for _, node := range request.Nodes {
					if ok, err := checkAndAcceptNode(
						node,
						ctxt); err != nil || !ok {
						acceptFailedNodes = append(acceptFailedNodes, node)
					}
				}
				if len(acceptFailedNodes) > 0 {
					util.FailTask(
						"",
						fmt.Errorf(
							"%s-Failed to accept nodes: %v",
							ctxt,
							acceptFailedNodes),
						t)
					return
				}

				provider := a.getProviderFromClusterType(request.ClusterType)
				if provider == nil {
					util.FailTask(
						fmt.Sprintf(
							"Error getting provider for cluster type: %s",
							request.ClusterType),
						fmt.Errorf("%s-%v", ctxt, err),
						t)
					return
				}
				err = provider.Client.Call(
					fmt.Sprintf("%s.%s", provider.Name, "ImportCluster"),
					models.RpcRequest{
						RpcRequestVars:    vars,
						RpcRequestData:    body,
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
							// Setup monitoring for the imported cluster
							t.UpdateStatus("Enabling/Scheduling monitoring for the cluster")
							if err := setupMonitoring(request.BootstrapNode); err != nil {
								logger.Get().Warning(
									"%s-Failed to setup monitoring for the cluster. error: %v",
									ctxt,
									err)
							}
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
		models.ENGINE_NAME,
		fmt.Sprintf("Import Cluster"),
		asyncTask,
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

func checkAndAcceptNode(node string, ctxt string) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var fetchedNode models.Node
	if err := coll.Find(bson.M{"hostname": node}).One(&fetchedNode); err != nil {
		return false, fmt.Errorf(
			"Node %s is not known to system. error: %v",
			node,
			err)
	} else {
		if fetchedNode.State == models.NODE_STATE_UNACCEPTED {
			fingerprint, err := GetCoreNodeManager().GetFingerPrint(node, ctxt)
			if err != nil {
				return false, fmt.Errorf(
					"Error getting fingerprint of node: %s. error: %v",
					node,
					err)
			}
			if ok, err := GetCoreNodeManager().AcceptNode(
				node,
				fingerprint,
				ctxt); err != nil || !ok {
				return false, fmt.Errorf(
					"Error accepting node: %s. error: %v",
					node,
					err)
			}
			// Change the node state to initializing
			if err := coll.Update(
				bson.M{"hostname": node},
				bson.M{"$set": bson.M{"state": models.NODE_STATE_INITIALIZING}}); err != nil {
				return false, fmt.Errorf(
					"Error changing the state if node: %s. error: %v",
					node,
					err)
			}
			go Check_status(node, ctxt)
		}
	}
	return true, nil
}

func setupMonitoring(bootstrapNode string) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	if err := coll.Find(bson.M{"hostname": bootstrapNode}).One(&node); err != nil {
		return err
	}
	collCluster := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collCluster.Find(bson.M{"clusterid": node.ClusterId}).One(&cluster); err != nil {
		return err
	}

	var monitoringState models.MonitoringState
	monitoringState.Plugins = append(cluster.Monitoring.Plugins, monitoring.GetDefaultThresholdValues()...)
	if err := updatePluginsInDb(bson.M{"name": cluster.Name}, monitoringState); err != nil {
		return err
	}

	ScheduleCluster(cluster.ClusterId, cluster.MonitoringInterval)

	return nil
}
