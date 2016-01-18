package monitoring

import (
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/schedule"
	"github.com/skyrings/skyring/tools/uuid"
)

//In memory ClusterId to ScheduleId map
var ClusterMonitoringSchedules map[uuid.UUID]uuid.UUID

func InitSchedules() {
	app := skyring.GetApp()
	clusters, err := GetClusters()
	if err != nil {
		logger.Get().Error("Error getting the clusters list: %v", err)
		return
	}
	for _, cluster := range clusters {
		go MonitorCluster(cluster.ClusterId, cluster.MonitoringInterval)
	}
}

func ScheduleCluster(clusterId uuid.UUID, intervalInSecs int) {
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		logger.Get().Error(err)
		return
	}
	f := MonitorCluster
	scheduler.Schedule(intervalInSecs*time.Second, f, map[string]interface{}{"clusterId": clusterId})
}

func MonitorCluster(params map[string]interface{}) {
	clusterId := params["clusterId"]
	if err, metrics := app.RouteProviderBasedMonitoring(clusterId); err != nil {
		logger.Get().Error("Could not initiate monitoring cluster with id :%v", clusterId)
		return
	}
	if err = GetMonitoringManager().PushToDb(metrics); err != nil {
		logger.Get().Error(err)
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
