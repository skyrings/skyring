package monitoring

import (
	"github.com/skyrings/skyring/task"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/utils"
	"time"
	"os/exec"
	"monitor_gluster_cluster.go"
	"monitor_gluster_ceph.go"
	influxdb "github.com/influxdb/influxdb/client"
)

func Schedule(a time.Duration, function interface{}, args ...interface{}) {
	TaskManager taskManager := NewTaskManager()
	for {
		select {
			case time, ok := <-time.After(a):
					if ok {
						taskManager.Run(function, args)
					}
			case msg, ok := <-channel:
			//The channel in the above line needs to be global to the USM core so that any deletion of a cluster is notified so that the schedule is stopped.
					if ok {
						switch msg {
							/*
							    Need to explore if messages need to be handled separately.
								Return from the infinite loop; exit the schedule if USM stopped
							*/
							default:
								return
						}
					}
			}
	}
}

//Need to see if this can be generalized by taking the set of tags and sub tags. Assuming all monitoring plugins to return results in a fixed format.
func EvaluateReturnValue(returnValue map[string][]string, resource_cluster_name string, resource_type string) influxdb.Point {
	// fetch from db the name of resource using resource id
	resource_name := "vol1"
	series_name := resource_cluster_name + "." + resource_type + "." + resource_name
	return influxdb.Point {
	}
}

func GetCommandString(commandTemplate string, data interface{}) string {
	var templateBuffer bytes.Buffer
	return template.Must(template.New("").Parse(commandTemplate)).Execute(&templateBuffer, data)
}

func PushToTimeSeriesDB(points influxdb.Point) {
	bps := influxdb.BatchPoints{
		Points: points,
		Database: conf.SystemConfig.TimeSeriesDBConfig.Database,
	}
	_, err := db.GetMonitoringDBClient().Write(bps)
	if err != nil {
        log.Fatal(err)
    }
}

func Init() {
    // This is just a one time run whenever USM is restarted
	// fetch list of clusters from db
	 //clusters := []
	 //for index, cluster : range clusters {
			//if cluster_type == "Gluster" {
			//May Be use reflection here to invoke function with name Monitor<cluster_type> with cluster as parameter
			//monitor_gluster := MonitorGluster
			//go Schedule(cluster.monitoring_interval, monitor_gluster, &cluster)
			//}
			//if cluster_type == "Ceph" {
				//go MonitorCeph(cluster)
			//}
	 //}
}

func get_cluster_list() *Cluster {
// Return list of clusters in USM
}

func Monitor(command string, target string) bool {
	returnVal := util.PyExecuteSaltCommandsOnTarget([command], target)
	PushToTimeSeriesDb(EvluateReturnValue(returnVal, ))
}
