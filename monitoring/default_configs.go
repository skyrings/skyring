package monitoring

func GetDefaultThresholdValues() (plugins []Plugin) {
	return []Plugin{
		{
			Name:   "df",
			Enable: true,
			Configs: []PluginConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
		{
			Name:   "memory",
			Enable: true,
			Configs: []PluginConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
		{
			Name:   "cpu",
			Enable: true,
			Configs: []PluginConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
	}
}

var DefaultClusterMonitoringInterval = 10
