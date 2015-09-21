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

type AuthConfig struct {
	ProviderName string
	ConfigFile   string
}

type SkyringCollection struct {
	Config         SkyringConfig
	Logging        SkyringLogging
	Authentication AuthConfig
}

func LoadAppConfiguration(configFilePath string) *SkyringCollection {
	var data SkyringCollection
	file, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		glog.Fatalf("Error Reading SkyRing Config: %s", err)
		panic(fmt.Sprintf("Error Reading SkyRing Config : %s", err))
	}
	err = json.Unmarshal(file, &data)
	if err != nil {
		glog.Fatalf("Error Unmarshalling SkyRing Config: %s", err)
		panic(fmt.Sprintf("Error Unmarshalling SkyRing Config: %s", err))
	}
	return &data

}
