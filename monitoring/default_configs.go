package monitoring

import (
	"github.com/skyrings/skyring-common/models"
)

func GetDefaultThresholdValues() (plugins []models.Plugin) {
	return []models.Plugin{
		{
			Name:   "df",
			Enable: true,
			Configs: []models.PluginConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
		{
			Name:   "memory",
			Enable: true,
			Configs: []models.PluginConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
		{
			Name:   "cpu",
			Enable: true,
			Configs: []models.PluginConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
	}
}
