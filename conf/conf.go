/*Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package conf

import (
	"encoding/json"
	"github.com/golang/glog"
	"io/ioutil"
)

type SkyringConfig struct {
	ConfigFilePath string
	HttpPort       int
}

type SkyringLogging struct {
	Logtostderr bool
	Log_dir     string
	V           int
	vmodule     string
}

type PluginConfig struct {
	Name           string
	PluginBinary   string
	ConfigFilePath string
}

type SkyringCollection struct {
	Config  SkyringConfig
	Logging SkyringLogging
}

type PluginCollection struct {
	Plugins       []PluginConfig
	UrlConfigPath string
}

func LoadAppConfiguration(configFilePath string) *SkyringCollection {
	var data SkyringCollection
	file, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		glog.Fatalf("Error Reading App Config: %s", err)
	}
	err = json.Unmarshal(file, &data)
	if err != nil {
		glog.Fatalf("Error Unmarshalling App Config: %s", err)
	}
	return &data

}

func LoadPluginConfiguration(configFilePath string) *PluginCollection {
	var data PluginCollection
	file, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		glog.Fatalf("Error Reading Plugin Config: %s", err)
	}
	err = json.Unmarshal(file, &data)
	if err != nil {
		glog.Fatalf("Error Unmarshalling plugin Config: %s", err)
	}
	return &data

}
