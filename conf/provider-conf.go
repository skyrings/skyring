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
	"github.com/op/go-logging"
	"github.com/skyrings/skyring/tools/logger"
	"io/ioutil"
	"path"
)

log := logger.Get()

type Route struct {
	Name       string `json:"name"`
	Method     string `json:"method"`
	Pattern    string `json:"pattern"`
	PluginFunc string `json:"pluginFunc"`
	Version    int    `json:"version"`
}

type ProviderConfig struct {
	Name           string `json:"name"`
	ProviderBinary string `json:"binary"`
}

type ProviderInfo struct {
	Provider ProviderConfig `json:"provider"`
	Routes   []Route        `json:"routes"`
}

func LoadProviderConfig(providerConfigDir string) []ProviderInfo {
	var (
		data       ProviderInfo
		collection []ProviderInfo
	)

	files, err := ioutil.ReadDir(providerConfigDir)
	if err != nil {
		log.Error("Unable to read directory: %s", err)
		log.Error("Failed to Initialize")
		return collection
	}
	for _, f := range files {
		log.Debug("File Name:", f.Name())

		file, err := ioutil.ReadFile(path.Join(providerConfigDir, f.Name()))
		if err != nil {
			log.Critical("Error Reading Config: %s", err)
			continue
		}
		err = json.Unmarshal(file, &data)
		if err != nil {
			log.Critical("Error Unmarshalling Config: %s", err)
			continue
		}
		collection = append(collection, data)
		data = ProviderInfo{}
	}
	log.Debug("Collection:", collection)
	return collection

}
