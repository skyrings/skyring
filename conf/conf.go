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
	"fmt"
	"io/ioutil"
)

type Config struct {
	Name           string
	ConfigFilePath string
}
type SwaggerConfig struct {
	Status          bool
	WebServicesUrl  string
	ApiPath         string
	SwaggerPath     string
	SwaggerFilePath string
}
type AppCollection struct {
	Apps    []Config
	Swagger SwaggerConfig
}

type PluginCollection struct {
	Plugins []Config
}

func LoadAppConfiguration(configFilePath string) *AppCollection {
	fmt.Println("In LoadAppConfiguration")
	var data AppCollection
	file, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		//log.Fatal(err)
	}
	err = json.Unmarshal(file, &data)
	if err != nil {
		//log.Fatal(err)
	}
	//fmt.Println(data)
	return &data

}

func LoadPluginConfiguration(configFilePath string) *PluginCollection {
	fmt.Println("In LoadPluginConfiguration")
	var data PluginCollection
	file, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		//log.Fatal(err)
	}
	err = json.Unmarshal(file, &data)
	if err != nil {
		//log.Fatal(err)
	}
	//fmt.Println(data)
	return &data

}
