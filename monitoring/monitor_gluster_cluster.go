package monitoring

//import "time"

//Whenever a new gluster_cluster is created schedule this function with its configured interval

/*
func MonitorGlusterCluster(cluster *Cluster) {
	//fetch list of volumes in current cluster
	for _, volume := range volumes {
		go MonitorVolume(volume, cluster)
	}
}

func random(max int) int {
	return rand.Intn(max) + min
}

func MonitorVolume(volume interface{}, cluster interface{}) {
	hosts = fetch list of nodes that are up in cluster
	rand.Seed(time.Now().Unix())
	target = hosts[random(len(hosts) - 1)]
	PushToTimeSeriesDB(EvaluateReturnValue(util.PyGetVolumeUtilization(target, volume.Name), resource_cluster_name, resource_type, resource_name))
}

func (scheduler *Scheduler) MonitorNewCluster(cluster interace{}, interval time.Duration) {
	getScheduler().Schedule(interval, MonitorGlusterCluster, cluster)
}
*/
