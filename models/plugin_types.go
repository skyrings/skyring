package models

import (
	"encoding/json"
	"fmt"
)

type Plugin string
type PluginThresholdMap map[Plugin]map[ThresholdType]int
type ThresholdType string

const (
	dfDefault int = 80
	networkDefault int = 75
	cpuDefault int = 60
	memoryDefault int = 80
)

const (
	Warning ThresholdType = "WarningMax"
	Critical ThresholdType = "FailureMax"
)

const (
	df     Plugin = "df"
	memory Plugin = "memory"
	cpu    Plugin = "cpu"
	network Plugin = "interface"
)

func (p *Plugin) GetDefaultThresholdValues() (PluginThresholdMap) {
	return PluginThresholdMap {
		df: {Critical: dfDefault},
		memory: {Critical: memoryDefault},
		cpu: {Critical: cpuDefault},
	}
}

var plugins map[string]Plugin = map[string]Plugin {
	"df": df, 
	"memory": memory, 
	"cpu": cpu,
}

func (p *Plugin) GetValues() (pluginNames []string) {
	for key, _ := range plugins {
		pluginNames = append(pluginNames, key)
	}
	return pluginNames
}

func (p *PluginThresholdMap) UnmarshalJSON(data []byte) (err error) {
	var objmap map[string]map[string]int
	if err := json.Unmarshal(data, &objmap); err == nil {
		for k, val := range objmap {
			i, ok := plugins[k]
			if !ok {
				return fmt.Errorf("Invalid key %s", k)
			}
			(*p)[i]= val
		}
		return nil
	}
	return err
}
