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
package nodemanager

import (
	"fmt"
	"github.com/skyrings/skyring-common/tools/logger"
	"io"
	"os"
	"sync"
)

type NodeManager struct {
	Name    string
	Manager NodeManagerInterface
}

type NodeManagersFactory func(config io.Reader) (NodeManagerInterface, error)

var (
	nodeManagersMutex sync.Mutex
	nodeManagers      = make(map[string]NodeManagersFactory)
)

func RegisterNodeManager(name string, factory NodeManagersFactory) {
	nodeManagersMutex.Lock()
	defer nodeManagersMutex.Unlock()

	if _, found := nodeManagers[name]; found {
		logger.Get().Info("Node manager already registered. Returing..")
		return
	}

	nodeManagers[name] = factory
}

func GetNodeManager(name string, config io.Reader) (NodeManagerInterface, error) {
	nodeManagersMutex.Lock()
	defer nodeManagersMutex.Unlock()

	factory_func, found := nodeManagers[name]
	if !found {
		logger.Get().Info("Node manager not found", name)
		return nil, nil
	}

	return factory_func(config)
}

func InitNodeManager(name string, configPath string) (NodeManagerInterface, error) {
	var manager NodeManagerInterface

	if name == "" {
		return nil, nil
	}

	var err error
	if configPath != "" {
		config, err := os.Open(configPath)
		if err != nil {
			logger.Get().Info("Couldnt open node manager config file", configPath)
			return nil, nil
		}

		defer config.Close()
		manager, err = GetNodeManager(name, config)
	} else {
		manager, err = GetNodeManager(name, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("Could not initialize node manager %s: %v", name, err)
	}
	if manager == nil {
		return nil, fmt.Errorf("Unknown node manager %s", name)
	}

	return manager, nil
}
