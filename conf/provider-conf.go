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

type Route struct {
	Name       string
	Method     string
	Pattern    string
	PluginFunc string
}

type PluginConfig struct {
	Name         string
	PluginBinary string
}

type RouteCollection struct {
	Plugin        PluginConfig
	Plugin_config map[string]interface{}
	Routes        []Route
}

func LoadProviderConfig(configFilePath string) *RouteCollection {
	var data RouteCollection
	file, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		glog.Fatalf("Error Reading URL Config:", err)
	}
	err = json.Unmarshal(file, &data)
	if err != nil {
		glog.Fatalf("Error Unmarshalling URL Config:", err)
	}
	return &data

}
