package monitoring

import (
	"github.com/skyrings/skyring-common/models"
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

func ValidPlugin(p models.Plugin) bool {
	validPluginName := Contains(p.Name, SupportedMonitoringPlugins)
	if validPluginName {
		for _, config := range p.Configs {
			if !ValidPluginConfig(config) {
				return false
			}
		}
		return true
	}
	return false
}

func ValidConfigType(c models.PluginConfig) bool {
	switch c.Category {
	case "threshold":
		return c.Type != "" && Contains(c.Type, SupportedThresholdTypes)
	}
	return true
}

func ValidConfigCategory(c models.PluginConfig) bool {
	return Contains(c.Category, SupportedConfigCategories)
}

func ValidPluginConfig(c models.PluginConfig) bool {
	return ValidConfigCategory(c) && ValidConfigType(c)
}

/*type collectd_config PluginConfig
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
}*/
