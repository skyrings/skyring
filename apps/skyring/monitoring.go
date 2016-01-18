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
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/schedule"
	"github.com/skyrings/skyring/tools/uuid"
	"github.com/skyrings/skyring/utils"
	"net/http"
	"strings"
	"time"
)

func (a *App) GET_Utilization(w http.ResponseWriter, r *http.Request) {
	var start_time string
	var end_time string
	var interval string
	vars := mux.Vars(r)
	node_id_str := vars["node-id"]
	node_id, _ := uuid.Parse(node_id_str)

	params := r.URL.Query()
	resource_name := params.Get("resource")
	duration := params.Get("duration")
	//	duration := params.Get("duration")

	storage_node := GetNode(*node_id)
	if storage_node.Hostname == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Node not found")
		logger.Get().Error("Node: %v not found", *node_id)
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

	//res, err := queryDB(query_cmd)
	res, err := GetMonitoringManager().QueryDB(map[string]interface{}{"nodename": storage_node.Hostname, "resource": resource_name, "start_time": start_time, "end_time": end_time, "interval": interval})
	if err == nil {
		json.NewEncoder(w).Encode(res)
	} else {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}

//In memory ClusterId to ScheduleId map
var ClusterMonitoringSchedules map[uuid.UUID]uuid.UUID

func InitSchedules() {
	clusters, err := GetClusters()
	if err != nil {
		logger.Get().Error("Error getting the clusters list: %v", err)
		return
	}
	for _, cluster := range clusters {
		go ScheduleCluster(cluster.ClusterId, cluster.MonitoringInterval)
	}
}

func ScheduleCluster(clusterId uuid.UUID, intervalInSecs int) {
	if intervalInSecs == 0 {
		intervalInSecs = monitoring.DefaultClusterMonitoringInterval
	}
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		logger.Get().Error(err.Error())
		return
	}
	f := GetApp().MonitorCluster
	if ClusterMonitoringSchedules == nil {
		ClusterMonitoringSchedules = make(map[uuid.UUID]uuid.UUID)
	}
	ClusterMonitoringSchedules[clusterId] = scheduler.Id
	scheduler.Schedule(time.Duration(intervalInSecs)*time.Second, f, map[string]interface{}{"clusterId": clusterId})
}

func DeleteClusterSchedule(clusterId uuid.UUID) {
	schedulerId := ClusterMonitoringSchedules[clusterId]
	schedule.DeleteScheduler(schedulerId)
	delete(ClusterMonitoringSchedules, clusterId)
}

func (a *App) MonitorCluster(params map[string]interface{}) {
	clusterId := params["clusterId"]
	id, ok := clusterId.(uuid.UUID)
	if !ok {
		logger.Get().Error("Failed to parse uuid")
		return
	}
	err, metrics := a.RouteProviderBasedMonitoring(id)
	if err != nil {
		logger.Get().Error("Could not initiate monitoring cluster with id :%v", id)
		return
	}
	if err = GetMonitoringManager().PushToDb(metrics); err != nil {
		logger.Get().Error(err.Error())
	}
	return
}

func GetClusters() (models.Clusters, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.Clusters
	err := collection.Find(nil).All(&clusters)
	return clusters, err
}
