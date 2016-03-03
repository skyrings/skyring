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
	"github.com/skyrings/skyring-common/tools/schedule"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring-common/utils"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

func getEntityName(entity_type string, entity_id uuid.UUID, parentId *uuid.UUID) (string, error) {
	switch entity_type {
	case monitoring.NODE:
		entity, entityFetchErr := GetNode(entity_id)
		if entityFetchErr != nil {
			return "", fmt.Errorf("Unknown %v with id %v.Err %v", entity_type, entity_id, entityFetchErr)
		}
		return entity.Hostname, nil
	case models.CLUSTER:
		entity, entityFetchErr := GetCluster(&entity_id)
		if entityFetchErr != nil {
			return "", fmt.Errorf("Unknown %v with id %v.Err %v", entity_type, entity_id, entityFetchErr)
		}
		return entity.Name, nil
	case monitoring.SLU:
		entity, entityFetchErr := GetSLU(parentId, entity_id)
		if entityFetchErr != nil {
			return "", fmt.Errorf("%v not a valid id of %v.Err %v", entity_id, entity_type, entityFetchErr)
		}
		return entity.Name, nil
	}
	return "", fmt.Errorf("Unsupported entity type %v", entity_type)
}

var entityParentMap = map[string]string{
	monitoring.SLU: models.CLUSTER,
}

func getParentName(queriedEntityType string, parentId uuid.UUID) (string, error) {
	switch queriedEntityType {
	case monitoring.SLU:
		parent, parentFetchErr := GetCluster(&parentId)
		if parentFetchErr != nil {
			return "", fmt.Errorf("%v not a valid id of %v.Error %v", parentId, models.CLUSTER, parentFetchErr)
		}
		return parent.Name, nil
	}
	return "", nil
}

func (a *App) GET_Utilization(w http.ResponseWriter, r *http.Request) {
	var start_time string
	var end_time string
	var interval string
	vars := mux.Vars(r)

	entity_id_str := vars["entity-id"]
	entity_type := vars["entity-type"]

	params := r.URL.Query()
	resource_name := params.Get("resource")
	duration := params.Get("duration")
	parent_id_str := params.Get("parent_id")

	entity_id, entityIdParseError := uuid.Parse(entity_id_str)
	if entityIdParseError != nil {
		HttpResponse(w, http.StatusBadRequest, entityIdParseError.Error())
		logger.Get().Error(entityIdParseError.Error())
		return
	}

	var parent_id *uuid.UUID
	var parentError error
	var parentName string
	if parent_id_str != "" {
		parent_id, parentError = uuid.Parse(parent_id_str)
		if parentError != nil {
			HttpResponse(w, http.StatusBadRequest, parentError.Error())
			logger.Get().Error(parentError.Error())
			return
		}

		parentName, parentError = getParentName(entity_type, *parent_id)
		if parentError != nil {
			HttpResponse(w, http.StatusBadRequest, parentError.Error())
			logger.Get().Error(parentError.Error())
			return
		}
	}

	entityName, entityNameError := getEntityName(entity_type, *entity_id, parent_id)
	if entityNameError != nil {
		HttpResponse(w, http.StatusBadRequest, entityNameError.Error())
		logger.Get().Error(entityNameError.Error())
		return
	}

	if duration != "" {
		if strings.Contains(duration, ",") {
			splt := strings.Split(duration, ",")
			start_time = splt[0]
			end_time = splt[1]
		} else {
			interval = duration
		}
	}

	paramsToQuery := map[string]interface{}{"nodename": entityName, "resource": resource_name, "start_time": start_time, "end_time": end_time, "interval": interval}
	if parentName != "" {
		paramsToQuery["parentName"] = parentName
	}

	res, err := GetMonitoringManager().QueryDB(paramsToQuery)
	if err == nil {
		json.NewEncoder(w).Encode(res)
	} else {
		HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}

//In memory ClusterId to ScheduleId map
var ClusterMonitoringSchedules map[uuid.UUID]uuid.UUID

func InitMonitoringSchedules() {
	if ClusterMonitoringSchedules == nil {
		ClusterMonitoringSchedules = make(map[uuid.UUID]uuid.UUID)
	}
	clusters, err := GetClusters()
	if err != nil {
		logger.Get().Error("Error getting the clusters list: %v", err)
		return
	}
	for _, cluster := range clusters {
		ScheduleCluster(cluster.ClusterId, cluster.MonitoringInterval)
	}
}

var mutex sync.Mutex

func SynchroniseScheduleMaintainers(clusterId uuid.UUID) (schedule.Scheduler, error) {
	mutex.Lock()
	defer mutex.Unlock()
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		return scheduler, err
	}
	ClusterMonitoringSchedules[clusterId] = scheduler.Id
	return scheduler, nil
}

func ScheduleCluster(clusterId uuid.UUID, intervalInSecs int) {
	if intervalInSecs == 0 {
		intervalInSecs = monitoring.DefaultClusterMonitoringInterval
	}
	scheduler, err := SynchroniseScheduleMaintainers(clusterId)
	if err != nil {
		logger.Get().Error(err.Error())
	}
	f := GetApp().MonitorCluster
	go scheduler.Schedule(time.Duration(intervalInSecs)*time.Second, f, map[string]interface{}{"clusterId": clusterId})
}

func DeleteClusterSchedule(clusterId uuid.UUID) {
	mutex.Lock()
	defer mutex.Unlock()
	schedulerId, ok := ClusterMonitoringSchedules[clusterId]
	if !ok {
		logger.Get().Error("Cluster with id %v not scheduled", clusterId)
		return
	}
	if err := schedule.DeleteScheduler(schedulerId); err != nil {
		logger.Get().Error("Failed to delete schedule for cluster %v.Error %v", clusterId, err)
	}
	delete(ClusterMonitoringSchedules, clusterId)
}

func (a *App) MonitorCluster(params map[string]interface{}) {
	clusterId := params["clusterId"]
	ctxt := fmt.Sprintf("%v", models.ENGINE_NAME)

	id, ok := clusterId.(uuid.UUID)
	if !ok {
		logger.Get().Error("%s - Failed to parse cluster id %v", ctxt, clusterId)
		return
	}

	go a.RouteProviderBasedMonitoring(id)

	cluster, clusterFetchError := GetCluster(&id)
	if clusterFetchError != nil {
		logger.Get().Error("%s - Unable to get cluster with id %v.Error %v", ctxt, id, clusterFetchError)
		return
	}

	nodes, nodesFetchError := getClusterNodesById(&id)
	if nodesFetchError != nil {
		logger.Get().Error("%s - Failed to fetch nodes in cluster %v. Err %v", ctxt, id, nodesFetchError.Error())
		return
	}

	hostname := conf.SystemConfig.TimeSeriesDBConfig.Hostname
	port := conf.SystemConfig.TimeSeriesDBConfig.DataPushPort

	var cluster_memory_used float64
	var cluster_memory_free float64

	var disk_reads float64
	var disk_writes float64

	for _, node := range nodes {
		/*
			Calculate Memory Used
		*/
		resource_name := monitoring.MEMORY + "." + monitoring.MEMORY + "-" + monitoring.USED
		mStatUsed, memoryUsedFetchError := GetMonitoringManager().GetInstantValue(node.Hostname, resource_name)
		if memoryUsedFetchError != nil {
			logger.Get().Error("%s - Error %v", ctxt, memoryUsedFetchError)
			continue
		}
		cluster_memory_used = cluster_memory_used + mStatUsed

		/*
			Calculate Free Memory
		*/
		resource_name = monitoring.MEMORY + "." + monitoring.MEMORY + "-" + monitoring.FREE
		mStatFree, memoryFreeFetchError := GetMonitoringManager().GetInstantValue(node.Hostname, resource_name)
		if memoryFreeFetchError != nil {
			logger.Get().Error("%s - Error %v", ctxt, memoryFreeFetchError)
			continue
		}
		cluster_memory_free = cluster_memory_free + mStatFree

		for _, disk := range node.StorageDisks {
			disk_name := strings.Replace(disk.Name, "/dev/", "", 1)
			resource_name = monitoring.DISK + "-" + disk_name + monitoring.DISK_IOPS
			diskReads, diskReadErr := GetMonitoringManager().GetInstantValue(node.Hostname, resource_name+monitoring.READ)
			if diskReadErr != nil {
				disk_reads = disk_reads + diskReads
			} else {
				logger.Get().Error("%s - Failed to fetch iops stats for %v of %v from cluster %v.Err %v", ctxt, disk.Name, node.Hostname, clusterId, diskReadErr)
			}

			diskWrites, diskWriteErr := GetMonitoringManager().GetInstantValue(node.Hostname, resource_name+monitoring.READ)
			if diskWriteErr != nil {
				disk_writes = disk_writes + diskWrites
			} else {
				logger.Get().Error("%s - Failed to fetch iops stats for %v of %v from cluster %v.Err %v", ctxt, disk.Name, node.Hostname, clusterId, diskWriteErr)
			}
		}
	}

	time_stamp_str := strconv.FormatInt(time.Now().Unix(), 10)

	table_name := conf.SystemConfig.TimeSeriesDBConfig.CollectionName + "." + cluster.Name + "."

	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.DISK + "-" + monitoring.READ: {time_stamp_str: strconv.FormatFloat(disk_reads, 'E', -1, 64)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing iops statistics for the cluster %v.Err %v", ctxt, clusterId, err)
	}

	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.DISK + "-" + monitoring.WRITE: {time_stamp_str: strconv.FormatFloat(disk_writes, 'E', -1, 64)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing iops statistics for the cluster %v.Err %v", ctxt, clusterId, err)
	}

	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.MEMORY + "-" + monitoring.USED_SPACE: {time_stamp_str: strconv.FormatFloat(cluster_memory_used, 'E', -1, 64)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing cluster memory utilization.Err %v", ctxt, err)
	}

	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.MEMORY + "-" + monitoring.FREE_SPACE: {time_stamp_str: strconv.FormatFloat(cluster_memory_free, 'E', -1, 64)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing cluster memory utilization.Err %v", ctxt, err)
	}
	net_memory_usage_percentage := strconv.FormatFloat(((cluster_memory_used * 100) / (cluster_memory_used + cluster_memory_free)), 'E', -1, 64)
	memory_percent_table := table_name + monitoring.MEMORY + "-" + monitoring.USAGE_PERCENT
	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{memory_percent_table: {time_stamp_str: net_memory_usage_percentage}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing cluster memory utilization.Err %v", ctxt, err)
	}

	return
}

func (a *App) POST_AddMonitoringPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error("Error parsing the cluster id: %s. error: %v", vars["cluster-id"], cluster_id_parse_error)
		HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
		return
	}
	var request monitoring.Plugin
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request. error: %v", err)
		HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Unable to unmarshal request. error: %v", err)
		HttpResponse(w, http.StatusBadRequest, "Unable to unmarshal request")
		return
	}
	cluster, clusterFetchErr := GetCluster(cluster_id)
	if clusterFetchErr != nil {
		logger.Get().Error("Failed to add monitoring configuration for cluster: %v.Error %v", *cluster_id, clusterFetchErr)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to add monitoring configuration for cluster: %v.Error %v", *cluster_id, clusterFetchErr))
		return
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	for _, plugin := range cluster.Monitoring.Plugins {
		if plugin.Name == request.Name {
			logger.Get().Error("Plugin %v already exists on cluster %v", request.Name, cluster.Name)
			HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Plugin %v already exists in cluster %v", request.Name, cluster.Name))
			return
		}
	}
	nodes, err := getClusterNodesById(cluster_id)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("Failed to get nodes for locking for cluster: %v.Error %v", *cluster_id, err))
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to get nodes for locking for cluster: %v.Error %v", *cluster_id, err))
		return
	}
	var cluster_node_names []string
	var down_nodes []string
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
		if node.Status == models.NODE_STATUS_ERROR {
			down_nodes = append(down_nodes, node.Hostname)
		}
	}
	if len(down_nodes) == len(cluster_node_names) {
		logger.Get().Error("All nodes in cluster %v are down", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in cluster %v are down", cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var monState models.MonitoringState
				var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(down_nodes))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
				monState.StaleNodes, _ = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())

				t.UpdateStatus("Started task to add the monitoring plugin : %v", t.ID)
				appLock, err := LockNodes(nodes, "POST_AddMonitoringPlugin")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				addNodeWiseErrors, addError := GetCoreNodeManager().AddMonitoringPlugin(cluster_node_names, "", request)
				if len(addNodeWiseErrors) != 0 {
					//The only error that GetMapKeys is if it doesn't get a map as param as it takes interface for the flexibility of handling any kind of input map
					// It is guranteed that AddMonitoringPlugin specifically returns a map else it would fail much before coming here.
					nodesInErrorValues, _ := util.GetMapKeys(addNodeWiseErrors)
					nodesInError := util.Stringify(nodesInErrorValues)
					if addError != nil {
						logger.Get().Error(addError.Error())
					}
					nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(nodesInError))
					staleMonitoringNodes, _ := util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
					monState.StaleNodes = staleMonitoringNodes
					logger.Get().Error("Failed to add monitoring configuration for the cluster %v. Error :%v", cluster.Name, addNodeWiseErrors)
					t.UpdateStatus("Failed to add monitoring configuration for the cluster %v. Error :%v", cluster.Name, addNodeWiseErrors)
				}
				updatedPlugins := append(cluster.Monitoring.Plugins, request)
				t.UpdateStatus("Updating the plugins to db")
				monState.Plugins = updatedPlugins
				if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id}, monState); dbError != nil {
					util.FailTask(fmt.Sprintf("Failed to add monitoring configuration for cluster: %v", *cluster_id), dbError, t)
					return
				}
				if len(monState.StaleNodes) == len(cluster_node_names) {
					util.FailTask(fmt.Sprintf("Failed to add monitoring configuration for cluster: %v", *cluster_id), fmt.Errorf("%v", addNodeWiseErrors), t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Create Cluster: %s", request.Name), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for adding monitoring plugin for cluster: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for add monitoring plugin")
		return
	} else {
		logger.Get().Debug("Task Created: %v for adding moniroring plugin for cluster: %v", taskId, *cluster_id)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func updatePluginsInDb(parameter bson.M, monitoringState models.MonitoringState) (err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	dbUpdateError := coll.Update(parameter, bson.M{"$set": bson.M{"monitoring": monitoringState}})
	return dbUpdateError
}

func (a *App) PUT_Thresholds(w http.ResponseWriter, r *http.Request) {
	var request []monitoring.Plugin = make([]monitoring.Plugin, 0)
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the threshold update request for the cluster %v. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, "Unable to parse the request")
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Unable to unmarshall the threshold update request of cluster %v. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal threshold update request on cluster %v. error: %v", cluster_id, err))
		return
	}
	cluster_id_uuid, cluster_id_parse_error := uuid.Parse(cluster_id)
	if cluster_id_parse_error != nil {
		logger.Get().Error("Failed to parse cluster id %v. error: %v", cluster_id, cluster_id_parse_error)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse cluster id %v. error: %v", cluster_id, cluster_id_parse_error))
		return
	}
	cluster, clusterFetchErr := GetCluster(cluster_id_uuid)
	if clusterFetchErr != nil {
		logger.Get().Error("Failed to get cluster with id %v. error: %v", cluster_id, clusterFetchErr)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to get cluster with id %v. error: %v", cluster_id, clusterFetchErr))
		return
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", cluster_id_uuid)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	nodes, err := getClusterNodesById(cluster_id_uuid)
	if err != nil {
		logger.Get().Error("Failed to get nodes of cluster id %v. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to get nodes of cluster id %v. error: %v", cluster_id, err))
		return
	}
	var cluster_node_names []string
	var down_nodes []string
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
		if node.Status == models.NODE_STATUS_ERROR {
			down_nodes = append(down_nodes, node.Hostname)
		}
	}
	if len(down_nodes) == len(cluster_node_names) {
		logger.Get().Error("All nodes in cluster %v are down", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in cluster %v are down", cluster.Name))
		return
	}
	if len(request) == 0 {
		logger.Get().Error("No thresholds passed for configuration in cluster %v", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("No thresholds passed for configuration in cluster %v", cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var monState models.MonitoringState
				var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(down_nodes))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
				monState.StaleNodes, _ = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
				t.UpdateStatus("Started task to update monitoring plugins configuration : %v", t.ID)
				var updatedPlugins []monitoring.Plugin
				var pluginUpdateError error
				appLock, err := LockNodes(nodes, "PUT_Thresholds")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				if updatedPlugins, pluginUpdateError = monitoring.UpdatePluginsConfigs(cluster.Monitoring.Plugins, request); pluginUpdateError != nil {
					util.FailTask(fmt.Sprintf("Failed to update thresholds for cluster: %v", *cluster_id_uuid), pluginUpdateError, t)
					return
				}
				updateConfigurationErrors, updateErr := GetCoreNodeManager().UpdateMonitoringConfiguration(cluster_node_names, request)
				if len(updateConfigurationErrors) != 0 {
					updateConfigurationErrorValues, _ := util.GetMapKeys(updateConfigurationErrors)
					updateFailedNodes := util.Stringify(updateConfigurationErrorValues)
					if updateErr != nil {
						logger.Get().Error("Failed to update thresholds for cluster: %v.Error: %v", *cluster_id_uuid, updateErr.Error())
					}
					nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(updateFailedNodes))
					staleMonitoringNodes, _ := util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
					monState.StaleNodes = staleMonitoringNodes
					logger.Get().Error("Failed to update monitoring configuration on %v of cluster %v", updateFailedNodes, cluster.Name)
					t.UpdateStatus("Failed to update monitoring configuration on %v of cluster %v", updateFailedNodes, cluster.Name)
				}
				monState.Plugins = updatedPlugins
				t.UpdateStatus("Updating new configuration to db")
				if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id_uuid}, monState); dbError != nil {
					util.FailTask(fmt.Sprintf("Failed to update thresholds for cluster: %v", *cluster_id_uuid), dbError, t)
					return
				}
				if len(monState.StaleNodes) == len(cluster_node_names) {
					util.FailTask(fmt.Sprintf("Failed to update thresholds for cluster: %v", *cluster_id_uuid), fmt.Errorf("%v", updateConfigurationErrors), t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
			}
			return
		}
	}
	if taskId, err := a.GetTaskManager().Run("Update monitoring plugins configuration", asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for update monitoring plugin configuration for cluster: %v. error: %v", *cluster_id_uuid, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for update monitoring plugin configuration")
		return
	} else {
		logger.Get().Debug("Task Created: %v for updating monitoring plugin thresholds for cluster: %v", taskId, cluster_id_uuid)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) POST_froceUpdateMonitoringConfiguration(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var cluster models.Cluster
	var err error
	cluster_id, err := uuid.Parse(vars["cluster-id"])
	if err != nil {
		logger.Get().Error(err.Error())
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cluster, err = GetCluster(cluster_id); err != nil {
		logger.Get().Error(err.Error())
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	if len(cluster.Monitoring.StaleNodes) == 0 {
		logger.Get().Error("All nodes in the cluster %v have fresh monitoring configurations", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in the cluster %v have fresh monitoring configurations", cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		if pluginUpdateErr := forceUpdatePlugins(cluster, cluster.Monitoring.StaleNodes); pluginUpdateErr != nil {
			util.FailTask(fmt.Sprintf("Failed to restore monitoring configuration on cluster %v", cluster.Name), pluginUpdateErr, t)
			return
		}
		t.UpdateStatus("Success")
		t.Done(models.TASK_STATUS_SUCCESS)
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Enforce monitoring plugins update"), asyncTask, 300*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for enforcing monitoring plugin update. error: %v", err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for enforcing monitoring plugin update")
		return
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func forceUpdatePlugins(cluster models.Cluster, nodes []string) error {
	var monState models.MonitoringState
	var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
	nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
	nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(nodes))
	monState.Plugins = cluster.Monitoring.Plugins
	plugin_names := make([]string, len(cluster.Monitoring.Plugins))
	for index, plugin := range cluster.Monitoring.Plugins {
		plugin_names[index] = plugin.Name
	}
	if forUpdateErrors, forceUpdatePythonError := GetCoreNodeManager().EnforceMonitoring(plugin_names, nodes, "", cluster.Monitoring.Plugins); len(forUpdateErrors) != 0 || forceUpdatePythonError != nil {
		if forceUpdatePythonError != nil {
			return fmt.Errorf("Failed to update monitoring configuration on nodes : %v of cluster: %v.Error: %v", nodes, cluster.Name, forceUpdatePythonError)
		}
		nodesInErrorValues, _ := util.GetMapKeys(forUpdateErrors)
		nodesInError := util.Stringify(nodesInErrorValues)

		nodesInSuccess := util.StringSetDiff(nodes, nodesInError)
		for _, nodeInSuccess := range util.GenerifyStringArr(nodesInSuccess) {
			nodesWithStaleMonitoringConfig.Remove(nodeInSuccess)
		}
		monState.StaleNodes, err = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())

		if dbError := updatePluginsInDb(bson.M{"clusterid": cluster.ClusterId}, monState); dbError != nil {
			if len(monState.StaleNodes) != 0 {
				return fmt.Errorf("Updating monitoring configuration failed on %v and failed to persist the failure to db.Error: %v", monState.StaleNodes, dbError)
			}
			return fmt.Errorf("Failed to update monitoring configuration to db.Error %v", dbError)
		}
		return nil
	}
	for _, nodeInSuccess := range nodes {
		nodesWithStaleMonitoringConfig.Remove(nodeInSuccess)
	}
	staleNodes, _ := util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
	monState.StaleNodes = staleNodes
	if dbError := updatePluginsInDb(bson.M{"clusterid": cluster.ClusterId}, monState); dbError != nil {
		return fmt.Errorf("Failed to update monitoring configuration to db for the cluster %v. Error %v", cluster.Name, dbError)
	}
	return nil
}

func monitoringPluginActivationDeactivations(enable bool, plugin_name string, cluster_id *uuid.UUID, w http.ResponseWriter, a *App) {
	var action string
	cluster, err := GetCluster(cluster_id)
	if err != nil {
		logger.Get().Error("Error getting cluster with id: %v. error: %v", *cluster_id, err)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", *cluster_id)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	plugin_index := -1
	for index, plugin := range cluster.Monitoring.Plugins {
		if plugin.Name == plugin_name {
			if plugin.Enable != enable {
				plugin_index = index
				break
			}
		}
	}
	if enable {
		action = "enable"
	} else {
		action = "disable"
	}
	if plugin_index == -1 {
		logger.Get().Error("Plugin is either already %vd or not configured on cluster: %s", action, cluster.Name)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Plugin is either already %vd or not configured", action))
		return
	}
	if !monitoring.Contains(plugin_name, monitoring.SupportedMonitoringPlugins) {
		logger.Get().Error("Unsupported plugin: %s for cluster: %s", plugin_name, cluster.Name)
		HttpResponse(w, http.StatusInternalServerError, "Unsupported plugin")
	}
	nodes, nodesFetchError := getClusterNodesById(cluster_id)
	if nodesFetchError != nil {
		logger.Get().Error("Unbale to get nodes for cluster: %v. error: %v", cluster.Name, nodesFetchError)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	var cluster_node_names []string
	var down_nodes []string
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
		if node.Status == models.NODE_STATUS_ERROR {
			down_nodes = append(down_nodes, node.Hostname)
		}
	}
	if len(down_nodes) == len(cluster_node_names) {
		logger.Get().Error("All nodes in cluster %v are down", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in cluster %v are down", cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var monState models.MonitoringState
				var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(down_nodes))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
				monState.StaleNodes, _ = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
				t.UpdateStatus("Started task to %v monitoring plugin : %v", action, t.ID)
				var actionNodeWiseFailure map[string]string
				var actionErr error
				appLock, err := LockNodes(nodes, "monitoringPluginActivationDeactivations")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				if enable {
					actionNodeWiseFailure, actionErr = GetCoreNodeManager().EnableMonitoringPlugin(cluster_node_names, plugin_name)
				} else {
					actionNodeWiseFailure, actionErr = GetCoreNodeManager().DisableMonitoringPlugin(cluster_node_names, plugin_name)
				}
				if len(actionNodeWiseFailure) != 0 {
					if actionErr != nil {
						logger.Get().Error(actionErr.Error())
					}
					//The only error that GetMapKeys is if it doesn't get a map as param as it takes interface for the flexibility of handling any kind of input map
					// It is guranteed that both EnableMonitoringPlugin and DisableMonitoringPlugin specifically returns a map else it would fail much before coming here.
					nodesInError, _ := util.GetMapKeys(actionNodeWiseFailure)
					updateFailedNodes := util.Stringify(nodesInError)
					nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(updateFailedNodes))
					staleMonitoringNodes, _ := util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
					monState.StaleNodes = staleMonitoringNodes
					logger.Get().Error("Failed to %v plugin on %v of cluster %v", action, updateFailedNodes, cluster.Name)
					t.UpdateStatus("Failed to %v plugin on %v of cluster %v", action, updateFailedNodes, cluster.Name)
				}
				index := monitoring.GetPluginIndex(plugin_name, cluster.Monitoring.Plugins)
				cluster.Monitoring.Plugins[index].Enable = enable
				t.UpdateStatus("Updating changes to db")
				monState.Plugins = cluster.Monitoring.Plugins
				if dbError := updatePluginsInDb(bson.M{"clusterid": cluster_id}, monState); dbError != nil {
					util.FailTask(fmt.Sprintf("Failed to %s plugin %s on cluster: %s", action, plugin_name, cluster.Name), dbError, t)
					return
				}
				if len(monState.StaleNodes) == len(cluster_node_names) {
					util.FailTask(fmt.Sprintf("Failed to %s plugin %s on cluster: %s", action, plugin_name, cluster.Name), fmt.Errorf("%v plugin %s failed on cluster %v", action, plugin_name, cluster.Name), t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("%s monitoring plugin: %s", action, plugin_name), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for %s monitoring plugin on cluster: %s. error: %v", action, cluster.Name, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for"+action+"monitoring plugin")
		return
	} else {
		logger.Get().Debug("Task Created: %v for %s monitoring plugin on cluster: %s", taskId, action, cluster.Name)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) POST_MonitoringPluginEnable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error("Error parsing request. error: %v", cluster_id_parse_error)
		HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	plugin_name := vars["plugin-name"]
	monitoringPluginActivationDeactivations(true, plugin_name, cluster_id, w, a)
}

func (a *App) POST_MonitoringPluginDisable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	cluster_id, cluster_id_parse_error := uuid.Parse(vars["cluster-id"])
	if cluster_id_parse_error != nil {
		logger.Get().Error("Error parsing the request. error: %v", cluster_id_parse_error)
		HttpResponse(w, http.StatusInternalServerError, cluster_id_parse_error.Error())
	}
	plugin_name := vars["plugin-name"]
	monitoringPluginActivationDeactivations(false, plugin_name, cluster_id, w, a)
}

func (a *App) REMOVE_MonitoringPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id := vars["cluster-id"]
	uuid, err := uuid.Parse(cluster_id)
	if err != nil {
		logger.Get().Error("Error parsing the request cluster id: %s. error: %v", cluster_id, err)
		HttpResponse(w, http.StatusMethodNotAllowed, err.Error())
		return
	}
	plugin_name := vars["plugin-name"]
	cluster, clusterFetchErr := GetCluster(uuid)
	if clusterFetchErr != nil {
		logger.Get().Error("Failed to remove plugin %s for cluster: %v.Error %v", *uuid, plugin_name, clusterFetchErr)
		HttpResponse(w, http.StatusMethodNotAllowed, fmt.Sprintf("Failed to remove plugin %s for cluster: %v.Error %v", *uuid, plugin_name, clusterFetchErr))
		return
	}

	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		logger.Get().Error("Cluster: %v is in un-managed state", *uuid)
		HttpResponse(w, http.StatusMethodNotAllowed, "Cluster is in un-managed state")
		return
	}

	nodes, nodesFetchError := getClusterNodesById(uuid)
	if nodesFetchError != nil {
		logger.Get().Error("Unbale to get nodes for cluster: %v. error: %v", *uuid, nodesFetchError)
		HttpResponse(w, http.StatusInternalServerError, nodesFetchError.Error())
	}
	var cluster_node_names []string
	var down_nodes []string
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
		if node.Status == models.NODE_STATUS_ERROR {
			down_nodes = append(down_nodes, node.Hostname)
		}
	}
	if len(down_nodes) == len(cluster_node_names) {
		logger.Get().Error("All nodes in cluster %v are down", cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("All nodes in cluster %v are down", cluster.Name))
		return
	}
	if monitoring.GetPluginIndex(plugin_name, cluster.Monitoring.Plugins) == -1 {
		logger.Get().Error("Plugin %v already deleted on cluster %v", plugin_name, cluster.Name)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Plugin %v already deleted on cluster %v", plugin_name, cluster.Name))
		return
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var monState models.MonitoringState
				var nodesWithStaleMonitoringConfig = util.NewSetWithType(reflect.TypeOf(""))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(cluster.Monitoring.StaleNodes))
				nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(down_nodes))
				monState.StaleNodes, _ = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())
				t.UpdateStatus("Task created to remove monitoring plugin %v", plugin_name)
				appLock, err := LockNodes(nodes, "REMOVE_MonitoringPlugin")
				if err != nil {
					util.FailTask("Failed to acquire lock", err, t)
					return
				}
				defer a.GetLockManager().ReleaseLock(*appLock)
				removeNodeWiseFailure, removeErr := GetCoreNodeManager().RemoveMonitoringPlugin(cluster_node_names, plugin_name)
				if len(removeNodeWiseFailure) != 0 || removeErr != nil {
					//The only error that GetMapKeys is if it doesn't get a map as param as it takes interface for the flexibility of handling any kind of input map
					// It is guranteed that RemoveMonitoringPlugin specifically returns a map else it would fail much before coming here.
					nodesInErrorValues, _ := util.GetMapKeys(removeNodeWiseFailure)
					nodesInError := util.Stringify(nodesInErrorValues)

					nodesWithStaleMonitoringConfig.AddAll(util.GenerifyStringArr(nodesInError))
					monState.StaleNodes, err = util.StringifyInterface(nodesWithStaleMonitoringConfig.GetElements())

					logger.Get().Error("Failed to remove plugin %v with error %v on cluster %v", plugin_name, removeNodeWiseFailure, cluster.Name)
					if removeErr != nil {
						logger.Get().Error(removeErr.Error())
					}
					t.UpdateStatus("Failed to remove plugin %v with error %v", plugin_name, removeNodeWiseFailure)
				}
				index := monitoring.GetPluginIndex(plugin_name, cluster.Monitoring.Plugins)
				updatedPlugins := append(cluster.Monitoring.Plugins[:index], cluster.Monitoring.Plugins[index+1:]...)
				monState.Plugins = updatedPlugins
				t.UpdateStatus("Updating the plugin %s removal to db", plugin_name)
				if dbError := updatePluginsInDb(bson.M{"clusterid": uuid}, monState); dbError != nil {
					util.FailTask(fmt.Sprintf("Failed to remove plugin %s for cluster: %v", *uuid, plugin_name), dbError, t)
					return
				}
				if len(monState.StaleNodes) == len(cluster_node_names) {
					util.FailTask(fmt.Sprintf("Failed to remove plugin %s for cluster: %v", *uuid, plugin_name), fmt.Errorf("%v", removeNodeWiseFailure), t)
					return
				}
				t.UpdateStatus("Success")
				t.Done(models.TASK_STATUS_SUCCESS)
				return
			}
		}
	}
	if taskId, err := a.GetTaskManager().Run(fmt.Sprintf("Remove monitoring plugin : %s", plugin_name), asyncTask, 120*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("Unable to create task for remove monitoring plugin for cluster: %v. error: %v", *uuid, err)
		HttpResponse(w, http.StatusInternalServerError, "Task creation failed for remove monitoring plugin")
		return
	} else {
		logger.Get().Debug("Task Created: %v for remove monitoring plugin for cluster: %v", taskId, *uuid)
		bytes, _ := json.Marshal(models.AsyncResponse{TaskId: taskId})
		w.WriteHeader(http.StatusAccepted)
		w.Write(bytes)
	}
}

func (a *App) GET_MonitoringPlugins(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		logger.Get().Error(fmt.Sprintf("Failed to parse the cluster id with error %v", err))
		return
	}
	cluster, err := GetCluster(cluster_id)
	if err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting cluster with id: %v. error: %v", *cluster_id, err))
		logger.Get().Error(fmt.Sprintf("Failed to fetch cluster with error %v", err))
		return
	}
	if cluster.Name == "" {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Cluster with id: %v not found", *cluster_id))
		logger.Get().Error("Cluster not found")
		return
	} else {
		json.NewEncoder(w).Encode(cluster.Monitoring.Plugins)
	}
}

func (a *App) Get_Summary(w http.ResponseWriter, r *http.Request) {
	var system models.System
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error Getting the context.Err %v", err))
		return
	}
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_SKYRING_UTILIZATION)
	if err := coll.Find(bson.M{"name": monitoring.SYSTEM}).One(&system); err != nil {
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Could not fetch summary.Err %v", err))
		logger.Get().Error(fmt.Sprintf("%s - Could not fetch summary.Err %v", ctxt, err))
		return
	}
	json.NewEncoder(w).Encode(system)
}

func (a *App) Get_ClusterSummary(w http.ResponseWriter, r *http.Request) {
	cSummary := models.ClusterSummary{}

	vars := mux.Vars(r)
	cluster_id_str := vars["cluster-id"]
	cluster_id, err := uuid.Parse(cluster_id_str)
	if err != nil {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing the cluster id: %s", cluster_id_str))
		logger.Get().Error(fmt.Sprintf("Failed to parse the cluster id with error %v", err))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error Getting the context.Err %v", err))
		return
	}

	cluster, clusterFetchErr := GetCluster(cluster_id)
	if clusterFetchErr != nil {
		logger.Get().Error("Unknown cluster with id %v.Err %v", cluster_id, clusterFetchErr)
		HttpResponse(w, http.StatusInternalServerError, fmt.Sprintf("Unknown cluster with id %v.Err %v", cluster_id, clusterFetchErr))
		return
	}

	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var poolsUsage []models.PoolUsage
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).Sort("-percentused").All(&poolsUsage); err != nil {
		logger.Get().Error("%s - Failed to fetch most used storages from cluster %v.Err %v", ctxt, *cluster_id, err)
	}
	if len(poolsUsage) > 5 {
		cSummary.MostUsedPools = poolsUsage[:4]
	} else {
		cSummary.MostUsedPools = poolsUsage
	}

	coll = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	var slus []models.StorageLogicalUnit
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&slus); err != nil {
		logger.Get().Error("%s - Failed to fetch storage logical units from cluster %v.Err %v", ctxt, *cluster_id, err)
	}
	slu_down_cnt := 0
	for _, slu := range slus {
		if slu.Status == models.SLU_STATUS_ERROR {
			slu_down_cnt = slu_down_cnt + 1
		}
	}
	cSummary.SLUCount = map[string]int{models.TOTAL: len(slus), models.SluStatuses[models.SLU_STATUS_ERROR]: slu_down_cnt}

	unmanagedNodes, unmanagedNodesError := GetCoreNodeManager().GetUnmanagedNodes()
	if unmanagedNodesError != nil {
		logger.Get().Error("%s - %s", ctxt, fmt.Sprintf("Failed to fetch unmanaged nodes.Err %v", unmanagedNodesError))
	}

	nodesInCluster, clusterNodesFetchError := getClusterNodesById(cluster_id)
	if clusterNodesFetchError != nil {
		logger.Get().Error("%s - %s", ctxt, fmt.Sprintf("Failed to fetch nodes of cluster.Err %v", cluster.Name))
	}
	/*
		Count the number of down nodes
	*/
	var error_nodes int
	for _, node := range nodesInCluster {
		if node.Status == models.NODE_STATUS_ERROR {
			error_nodes = error_nodes + 1
		}
	}

	cSummary.NodesCount = map[string]int{models.TOTAL: len(nodesInCluster), models.NodeStatuses[models.NODE_STATUS_ERROR]: error_nodes, models.NodeStates[models.NODE_STATE_UNACCEPTED]: len(*unmanagedNodes)}

	cSummary.Usage = cluster.Usage
	cSummary.StorageProfileUsage = cluster.StorageProfileUsage
	cSummary.ObjectCount = cluster.ObjectCount

	coll = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storages models.Storages
	if err := coll.Find(bson.M{"clusterid": *cluster_id}).All(&storages); err != nil {
		logger.Get().Error("%s - Error getting the storage list. error: %v", ctxt, err)
	}
	storage_down_cnt := 0
	for _, storage := range storages {
		if storage.Status == models.STORAGE_STATUS_ERROR {
			storage_down_cnt = storage_down_cnt + 1
		}
	}
	cSummary.StorageCount = map[string]int{models.TOTAL: len(storages), STORAGE_STATUS_DOWN: storage_down_cnt}

	json.NewEncoder(w).Encode(cSummary)
}

func Compute_System_Summary(p map[string]interface{}) {
	var system models.System

	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return
	}

	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())

	system.Name = monitoring.SYSTEM
	time_stamp_str := strconv.FormatInt(time.Now().Unix(), 10)
	table_name := conf.SystemConfig.TimeSeriesDBConfig.CollectionName + "." + models.SYSTEM + "."
	hostname := conf.SystemConfig.TimeSeriesDBConfig.Hostname
	port := conf.SystemConfig.TimeSeriesDBConfig.DataPushPort

	clusters, clusterFetchError := GetClusters()
	if clusterFetchError != nil {
		logger.Get().Error("%s - Failed to fetch clusters.Err %v", ctxt, clusterFetchError)
	}

	/*
		Count the number of unmanaged nodes
	*/
	unmanagedNodes, unmanagedNodesError := GetCoreNodeManager().GetUnmanagedNodes()
	if unmanagedNodesError != nil {
		logger.Get().Error("%s - %s", ctxt, fmt.Sprintf("Failed to fetch unmanaged nodes.Err %v", unmanagedNodesError))
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	var slus []models.StorageLogicalUnit
	if err := collection.Find(nil).All(&slus); err != nil {
		logger.Get().Error("%s - Error getting the slus list. error: %v", ctxt, err)
	}
	system.SLUCount = map[string]int{models.TOTAL: len(slus)}

	collection = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)
	var storages models.Storages
	if err := collection.Find(nil).All(&storages); err != nil {
		logger.Get().Error("%s - Error getting the storage list. error: %v", ctxt, err)
	}
	storage_down_cnt := 0
	for _, storage := range storages {
		if storage.Status == models.STORAGE_STATUS_ERROR {
			storage_down_cnt = storage_down_cnt + 1
		}
	}
	system.StorageCount = map[string]int{models.TOTAL: len(storages), STORAGE_STATUS_DOWN: storage_down_cnt}

	var net_cluster_used int64
	var net_cluster_total int64
	var net_memory_used float64
	var net_memory_free float64
	var total_nodes int
	var clusters_in_error int
	net_storage_profile_utilization := make(map[string]models.Utilization)
	error_nodes := 0
	for _, cluster := range clusters {
		if cluster.Status == models.CLUSTER_STATUS_ERROR {
			clusters_in_error = clusters_in_error + 1
		}
		nodesInCluster, clusterNodesFetchError := getClusterNodesById(&cluster.ClusterId)
		if clusterNodesFetchError != nil {
			logger.Get().Error("%s - %s", ctxt, fmt.Sprintf("Failed to fetch nodes of cluster.Err %v", cluster.Name))
			continue
		}

		/*
			Count the number of down nodes
		*/
		for _, node := range nodesInCluster {
			if node.Status == models.NODE_STATUS_ERROR {
				error_nodes = error_nodes + 1
			}
		}

		/*
			Count the total number of nodes
		*/
		total_nodes = total_nodes + len(nodesInCluster)
		/*
			Calculate net cluster utilization
		*/
		net_cluster_used = net_cluster_used + cluster.Usage.Used
		net_cluster_total = net_cluster_total + cluster.Usage.Total

		/*
			Calculate net storage profile utilization
		*/
		for profile, profileUtilization := range cluster.StorageProfileUsage {
			used := profileUtilization.Used
			total := profileUtilization.Total
			if utilization, ok := net_storage_profile_utilization[profile]; ok {
				used = used + utilization.Used
				total = total + utilization.Total
			}
			percentUsed := float64(used*100) / float64(total)
			net_storage_profile_utilization[profile] = models.Utilization{Used: used, Total: total, PercentUsed: percentUsed}
		}

		/*
			Calculate Memory Utilization
		*/
		resource_name := monitoring.MEMORY + "-" + monitoring.USED_SPACE
		mStatUsed, memoryUsedFetchError := GetMonitoringManager().GetInstantValue(cluster.Name, resource_name)
		if memoryUsedFetchError != nil {
			logger.Get().Error("%s - Error %v", ctxt, memoryUsedFetchError)
			continue
		}
		net_memory_used = net_memory_used + mStatUsed

		// Calculate Free Memory
		resource_name = monitoring.MEMORY + "-" + monitoring.FREE_SPACE
		mStatFree, memoryFreeFetchError := GetMonitoringManager().GetInstantValue(cluster.Name, resource_name)
		if memoryFreeFetchError != nil {
			logger.Get().Error("%s - Error %v", ctxt, memoryFreeFetchError)
			continue
		}
		net_memory_free = net_memory_free + mStatFree
	}
	system.ClustersCount = map[string]int{models.TOTAL: len(clusters), models.ClusterStatuses[models.CLUSTER_STATUS_ERROR]: clusters_in_error}

	system.NodesCount = map[string]int{models.TOTAL: total_nodes, models.NodeStatuses[models.NODE_STATUS_ERROR]: error_nodes, models.NodeStates[models.NODE_STATE_UNACCEPTED]: len(*unmanagedNodes)}

	// Update Cluster utilization to time series db
	percentSystemUsed := (float64(net_cluster_used*100) / float64(net_cluster_total))
	system.Usage = models.Utilization{Used: net_cluster_used, Total: net_cluster_total, PercentUsed: percentSystemUsed}
	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.USED_SPACE: {time_stamp_str: strconv.FormatInt(system.Usage.Used, 10)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing cluster utilization.Err %v", ctxt, err)
	}
	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.TOTAL_SPACE: {time_stamp_str: strconv.FormatInt(system.Usage.Total, 10)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing cluster utilization.Err %v", ctxt, err)
	}
	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.PERCENT_USED: {time_stamp_str: strconv.FormatFloat(percentSystemUsed, 'E', -1, 64)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing cluster utilization.Err %v", ctxt, err)
	}

	// Update memory utilization to time series db
	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.MEMORY + "-" + monitoring.FREE_SPACE: {time_stamp_str: strconv.FormatFloat(net_memory_free, 'E', -1, 64)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing memory utilization.Err %v", ctxt, err)
	}
	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{table_name + monitoring.MEMORY + "-" + monitoring.USED_SPACE: {time_stamp_str: strconv.FormatFloat(net_memory_used, 'E', -1, 64)}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing memory utilization.Err %v", ctxt, err)
	}
	memory_percent := strconv.FormatFloat(((net_memory_used * 100) / (net_memory_used + net_memory_free)), 'E', -1, 64)
	memory_percent_table := table_name + monitoring.MEMORY + "-" + monitoring.USAGE_PERCENT
	if err := GetMonitoringManager().PushToDb(map[string]map[string]string{memory_percent_table: {time_stamp_str: memory_percent}}, hostname, port); err != nil {
		logger.Get().Error("%s - Error pushing memory utilization.Err %v", ctxt, err)
	}
	system.StorageProfileUsage = net_storage_profile_utilization
	system.ProviderMonitoringDetails = make(map[string]map[string]interface{})
	otherProvidersDetails, otherDetailsFetchError := GetApp().FetchMonitoringDetailsFromProviders()
	if otherDetailsFetchError != nil {
		logger.Get().Error("%s - Error fetching the provider specific details. Error %v", ctxt, otherDetailsFetchError)
	} else {
		system.ProviderMonitoringDetails = otherProvidersDetails
	}

	/*
		Persist system into db
	*/
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_SKYRING_UTILIZATION)
	if _, err := coll.Upsert(bson.M{"name": monitoring.SYSTEM}, system); err != nil {
		logger.Get().Error("%s - Error persisting the system.Error %v", ctxt, err)
	}
}
