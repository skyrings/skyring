package backend

import (
	"encoding/json"
	"fmt"
)

type CollectdPlugin struct {
	Name    string           `json:"name"`
	Enable  bool             `json:"enable"`
	Configs []CollectdConfig `json:"configs"`
}

type CollectdConfig struct {
	Category string `json:"category"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

var SupportedConfigCategories = []string{
	"threshold",
	"interval",
	"miscellaneous",
}

var SupportedThresholdTypes = []string{
	"critical",
	"warning",
}

func Contains(key string, keys []string) bool {
	for _, permittedKey := range keys {
		if permittedKey == key {
			return true
		}
	}
	return false
}

func (p CollectdPlugin) Valid() bool {
	validPluginName := Contains(p.Name, SupportedCollectdPlugins)
	if validPluginName {
		for _, config := range p.Configs {
			if !config.Valid() {
				return false
			}
		}
		return true
	}
	return false
}

func (c CollectdConfig) IsConfigTypeValid() bool {
	switch c.Category {
	case "threshold":
		return c.Type != "" && Contains(c.Type, SupportedThresholdTypes)
	}
	return true
}

func (c CollectdConfig) IsConfigCategoryValid() bool {
	return Contains(c.Category, SupportedConfigCategories)
}

func (c CollectdConfig) Valid() bool {
	return c.IsConfigCategoryValid() && c.IsConfigTypeValid()
}

type collectd_config CollectdConfig
type collectd_plugin CollectdPlugin

var SupportedCollectdPlugins = []string{
	"df",
	"memory",
	"cpu",
	"python",
}

func (p *CollectdPlugin) UnmarshalJSON(data []byte) (err error) {
	tPlugin := collectd_plugin{}
	if err := json.Unmarshal(data, &tPlugin); err != nil {
		return err
	}
	if !(CollectdPlugin(tPlugin)).Valid() {
		return fmt.Errorf("Couldn't Parse %v", tPlugin)
	}
	*p = CollectdPlugin(tPlugin)
	fmt.Println(*p)
	return nil
}

func (c *CollectdConfig) UnmarshalJSON(data []byte) (err error) {
	tConfig := collectd_config{}
	if err := json.Unmarshal(data, &tConfig); err != nil {
		return err
	}
	if !(CollectdConfig(tConfig)).Valid() {
		return fmt.Errorf("Couldn't Parse %v", tConfig)
	}
	*c = CollectdConfig(tConfig)
	return nil
}

func UpdatePlugins(currentPlugins []CollectdPlugin, expectedPlugins []CollectdPlugin) []CollectdPlugin {
	var updated bool
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
			}
		}
	}
	return currentPlugins
}

func GetPluginIndex(pluginName string, plugins []CollectdPlugin) int {
	for index, plugin := range plugins {
		if plugin.Name == pluginName {
			return index
		}
	}
	return -1
}

func GetDefaultThresholdValues() (plugins []CollectdPlugin) {
	return []CollectdPlugin{
		{
			Name:   "df",
			Enable: true,
			Configs: []CollectdConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
		{
			Name:   "memory",
			Enable: true,
			Configs: []CollectdConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
		{
			Name:   "cpu",
			Enable: true,
			Configs: []CollectdConfig{
				{Category: "threshold", Type: "critical", Value: "70"},
			},
		},
	}
}

func ToSaltPillarCompat(plugins []CollectdPlugin) (saltPillar map[string]map[string]string) {
	saltPillar = make(map[string]map[string]string)
	var configMap map[string]string
	for _, plugin := range plugins {
		configMap = make(map[string]string)
		for _, config := range plugin.Configs {
			configMap[config.Type] = config.Value
		}
		saltPillar[plugin.Name] = configMap
	}
	fmt.Println(saltPillar)
	return saltPillar
}
