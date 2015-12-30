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
)

type SkyringConfig struct {
	Host              string `json:"host"`
	HttpPort          int    `json:"httpPort"`
	SupportedVersions []int  `json:"supportedversions"`
}

type SkyringLogging struct {
	LogToStderr bool          `json:"logToStderr"`
	Filename    string        `json:"filename"`
	Level       logging.Level `json:"level"`
}

type AuthConfig struct {
	ProviderName string
	ConfigFile   string
}

type SkyringCollection struct {
	Config               SkyringConfig     `json:"config"`
	Logging              SkyringLogging    `json:"logging"`
	NodeManagementConfig NodeManagerConfig `json:"nodemanagementconfig"`
	DBConfig             MongoDBConfig     `json:"dbconfig"`
	TimeSeriesDBConfig   InfluxDBconfig    `json:"timeseriesdbconfig"`
	Authentication       AuthConfig        `json:"authentication"`
}

type MongoDBConfig struct {
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type InfluxDBconfig struct {
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type NodeManagerConfig struct {
	ManagerName    string `json:"managername"`
	ConfigFilePath string `json:"configfilepath"`
}

var (
	SystemConfig SkyringCollection
)

func LoadAppConfiguration(configFilePath string) {
	file, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		logger.Get().Critical("Error Reading SkyRing Config: %s", err)
	}
	err = json.Unmarshal(file, &SystemConfig)
	if err != nil {
		logger.Get().Critical("Error Unmarshalling SkyRing Config: %s", err)
	}
}
