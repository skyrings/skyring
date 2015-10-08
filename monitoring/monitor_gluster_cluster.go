package monitoring

import (
	"github.com/skyrings/skyring/monitoring"
	"monitor_base.go"
	"text/template"
)

/*
	Map from
		resource_type -> {
			monitored_aspect -> {
				command_format
			}
		}
*/


var gluster_resource_monitor_command := map[string]string {
		"volume": "/usr/lib64/nagios/plugins/gluster/check_vol_utilization.py {{.vol_name}} -w {{.warning_limit}} -c {{.critical_limit}}"
		}

//Whenever a new gluster_cluster is created schedule this function with its configured interval
func MonitorGluster(cluster *Cluster) {
	volumes := []
	//Ideally target is a randomly chosen node from the set of up nodes
	target = "*"
	//fetch list of volumes
	for _, volume : range volumes {
		go MonitorVolume(volume, target)
	}
}

func MonitorVolume(volume *Volume, target string) {
	cmd_format := gluster_resource_monitor_command["volume"]
	cmd = GetCommandString(cmd_format, volume)
	Monitor(cmd, target)
}
