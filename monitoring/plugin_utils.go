package monitoring

import "fmt"

func UpdatePluginsConfigs(currentPlugins []Plugin, expectedPlugins []Plugin) ([]Plugin, error) {
	var updated bool
	newPluginExpected := true
	for _, ePlugin := range expectedPlugins {
		for cPluginIndex, cPlugin := range currentPlugins {
			if ePlugin.Name == cPlugin.Name {
				for _, eConfig := range ePlugin.Configs {
					for cConfigIndex, cConfig := range cPlugin.Configs {
						if eConfig.Type == cConfig.Type {
							updated = true
							currentPlugins[cPluginIndex].Configs[cConfigIndex] = eConfig
						}
					}
					if !updated {
						currentPlugins[cPluginIndex].Configs = append(currentPlugins[cPluginIndex].Configs, eConfig)
					}
					updated = false
				}
				newPluginExpected = false
			}
			if ePlugin.Name != cPlugin.Name && cPluginIndex == len(currentPlugins) {
				newPluginExpected = true
				break
			}
		}
		if newPluginExpected {
			return nil, fmt.Errorf("Cannot add plugin %s", ePlugin.Name)
		}
	}
	return currentPlugins, nil
}

func GetPluginIndex(pluginName string, plugins []Plugin) int {
	for index, plugin := range plugins {
		if plugin.Name == pluginName {
			return index
		}
	}
	return -1
}

func ToSaltPillarCompat(plugins []Plugin) (saltPillar map[string]map[string]string) {
	saltPillar = make(map[string]map[string]string)
	var configMap map[string]string
	for _, plugin := range plugins {
		configMap = make(map[string]string)
		for _, config := range plugin.Configs {
			configMap[config.Type] = config.Value
		}
		saltPillar[plugin.Name] = configMap
	}
	return saltPillar
}
