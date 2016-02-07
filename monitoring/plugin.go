package monitoring

import (
	"encoding/json"
	"fmt"
)

type Plugin struct {
	Name    string         `json:"name"`
	Enable  bool           `json:"enable"`
	Configs []PluginConfig `json:"configs"`
}

type PluginConfig struct {
	Category string `json:"category"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

const (
	CLUSTER_UTILIZATION = "cluster_utilization"
	SLU_UTILIZATION     = "slu_utilization"
	FREE_SPACE          = "free_bytes"
	USED_SPACE          = "used_bytes"
	PERCENT_USED        = "percent_bytes"
)

var (
	SupportedConfigCategories = []string{
		"threshold",
		"interval",
		"miscellaneous",
	}

	SupportedThresholdTypes = []string{
		"critical",
		"warning",
	}

	SupportedMonitoringPlugins = []string{
		"df",
		"memory",
		"cpu",
	}

	MonitoringWritePlugin = "dbpush"
)

func Contains(key string, keys []string) bool {
	for _, permittedKey := range keys {
		if permittedKey == key {
			return true
		}
	}
	return false
}

func (p Plugin) Valid() bool {
	validPluginName := Contains(p.Name, SupportedMonitoringPlugins)
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

func (c PluginConfig) ValidConfigType() bool {
	switch c.Category {
	case "threshold":
		return c.Type != "" && Contains(c.Type, SupportedThresholdTypes)
	}
	return true
}

func (c PluginConfig) ValidConfigCategory() bool {
	return Contains(c.Category, SupportedConfigCategories)
}

func (c PluginConfig) Valid() bool {
	return c.ValidConfigCategory() && c.ValidConfigType()
}

type collectd_config PluginConfig
type collectd_plugin Plugin

func (p *Plugin) UnmarshalJSON(data []byte) (err error) {
	tPlugin := collectd_plugin{}
	if err := json.Unmarshal(data, &tPlugin); err != nil {
		return err
	}
	if !(Plugin(tPlugin)).Valid() {
		return fmt.Errorf("Couldn't Parse %v", tPlugin)
	}
	*p = Plugin(tPlugin)
	return nil
}

func (c *PluginConfig) UnmarshalJSON(data []byte) (err error) {
	tConfig := collectd_config{}
	if err := json.Unmarshal(data, &tConfig); err != nil {
		return err
	}
	if !(PluginConfig(tConfig)).Valid() {
		return fmt.Errorf("Couldn't Parse %v", tConfig)
	}
	*c = PluginConfig(tConfig)
	return nil
}
