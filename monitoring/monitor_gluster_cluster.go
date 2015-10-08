package monitoring

/*
	Map from
		resource_type -> {
			monitored_aspect -> {
				command_format
			}
		}
*/

var gluster_resource_monitor_command map[string]string = map[string]string{
	"volume": "/usr/lib64/nagios/plugins/gluster/check_vol_utilization.py {{.vol_name}} -w {{.warning_limit}} -c {{.critical_limit}}",
}

//Whenever a new gluster_cluster is created schedule this function with its configured interval

/*
func MonitorGluster(cluster interface{}) {
//Ideally func MonitorGluster(cluster *Cluster) {
	//fetch list of volumes in current cluster
	//Ideally target is a randomly chosen node from the set of up nodes
	target = "*"
	//fetch list of volumes
	for _, volume := range volumes {
		clusterName := "" //Ideally name of cluster is fetched from cluster varialble passed
		go MonitorVolume(volume, target, clusterName)
	}
}

func MonitorVolume(volume *Volume, target, clusterName string) {
	cmd_format := gluster_resource_monitor_command["volume"]
	cmd = GetCommandString(cmd_format, volume)
	Monitor(cmd, target, clusterName, "volume", volume.Name)
}
*/
