package monitoring

import (
	"bytes"
	influxdb "github.com/influxdb/influxdb/client"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/task"
	"github.com/skyrings/skyring/utils"
	"log"
	"text/template"
	"time"
)

func Schedule(a time.Duration, channel chan string, function interface{}, args ...interface{}) {
	taskManager := task.NewTaskManager()
	for {
		select {
		case _, ok := <-time.After(a):
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
func EvaluateReturnValue(returnValue map[string][]string, resource_cluster_name, resource_type, resource_name string) []influxdb.Point {
	// fetch from db the name of resource using resource id
	series_name := resource_cluster_name + "." + resource_type + "." + resource_name
	//ToDo influxdb#Point generic generation
	var points []influxdb.Point
	point := influxdb.Point{Measurement: series_name, Time: time.Now(),
		Tags: map[string]string{
		//To Be explored if anything generic
		}, Precision: "s"}
	points = append(points, point)
	return points
}

func GetCommandString(commandTemplate string, data interface{}) string {
	var templateBuffer bytes.Buffer
	template.Must(template.New("").Parse(commandTemplate)).Execute(&templateBuffer, data)
	return templateBuffer.String()
}

func PushToTimeSeriesDB(points []influxdb.Point) bool {
	bps := influxdb.BatchPoints{
		Points:   points,
		Database: conf.SystemConfig.TimeSeriesDBConfig.Database,
	}
	_, err := db.GetMonitoringDBClient().Write(bps)
	if err != nil {
		log.Fatal(err)
		return false
	}
	return true
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

//func get_cluster_list() *Cluster {
//// Return list of clusters in USM
//}

func Monitor(command, target, resource_cluster_name, resource_type, resource_name string) bool {
	var cmds []string
	cmds = append(cmds, command)
	returnVal := util.PyExecuteSaltCommandsOnTarget(cmds, target)
	return PushToTimeSeriesDB(EvaluateReturnValue(returnVal, resource_cluster_name, resource_type, resource_name))
}
