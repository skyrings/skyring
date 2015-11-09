package backend

import (
	"encoding/json"
	"fmt"
)

type Plugin string
type PluginThresholdMap map[string]map[string]int

const (
	dfDefault      int = 80
	networkDefault int = 75
	cpuDefault     int = 60
	memoryDefault  int = 80
)

const (
	df      Plugin = "df"
	memory  Plugin = "memory"
	cpu     Plugin = "cpu"
	network Plugin = "interface"
	python  Plugin = "python"
)

func GetDefaultThresholdValues() PluginThresholdMap {
	return PluginThresholdMap{
		"df":     {"FailureMax": dfDefault},
		"memory": {"FailureMax": memoryDefault},
		"cpu":    {"FailureMax": cpuDefault},
	}
}

var Plugins = [...]string{
	"df",
	"memory",
	"cpu",
	"python",
}

func ContainsPlugin(pluginName string) bool {
	for _, plugin := range Plugins {
		if plugin == pluginName {
			return true
		}
	}
	return false
}

func Parse(mapToParse PluginThresholdMap) (err error) {
	for pluginName := range mapToParse {
		if ok := ContainsPlugin(pluginName); !ok {
			return fmt.Errorf("Invalid key %s", pluginName)
		}
	}
	return nil
}

func (p *PluginThresholdMap) UnmarshalJSON(data []byte) (err error) {
	var tMap map[string]map[string]int
	if err := json.Unmarshal(data, &tMap); err == nil {
		if err = Parse(tMap); err != nil {
			return err
		}
		*p = tMap
		return nil
	}
	return err
}
