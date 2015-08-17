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

package plugin

import (
	"fmt"
	"io"
	"net/rpc"
	"os"
	"sync"
)

type Plugin struct {
	Name   string
	Client *rpc.Client
}

/*
We have taken Kubernetes plugin architecture as a reference
https://github.com/kubernetes/kubernetes
*/

type PluginFactory func(config io.Reader) (ProviderInterface, error)

// All registered plugins
var pluginsMutex sync.Mutex
var plugins = make(map[string]PluginFactory)

//All initialized plugins
var initPluginMutex sync.Mutex
var initializedPlugins = make(map[string]ProviderInterface)

// RegisterPlugin registers a plugin by name.  This
// is expected to happen during app startup.
func RegisterPlugin(name string, factory PluginFactory) {
	pluginsMutex.Lock()
	defer pluginsMutex.Unlock()
	if _, found := plugins[name]; found {
		//glog.Fatalf("Cloud provider %q was registered twice", name)
	}
	//glog.V(1).Infof("Registered cloud provider %q", name)
	//var err error
	plugins[name] = factory
}

func GetPlugin(name string, config io.Reader) (ProviderInterface, error) {
	pluginsMutex.Lock()
	defer pluginsMutex.Unlock()
	f, found := plugins[name]
	if !found {
		fmt.Println("Plugin not found", name)
		return nil, nil
	}
	return f(config)
}

func GetInitializedPlugin(name string) (ProviderInterface, error) {
	initPluginMutex.Lock()
	defer initPluginMutex.Unlock()
	f, found := initializedPlugins[name]
	if !found {
		fmt.Println("Plugin not found initialized list")
		return nil, nil
	}
	return f, nil
}

// InitPlugin creates an instance of the named plugin.
func InitPlugin(name string, configFilePath string) (ProviderInterface, error) {
	var storageplugin ProviderInterface

	if name == "" {
		//glog.Info("No plugins specified.")
		return nil, nil
	}

	var err error
	storageplugin, err = GetInitializedPlugin(name)
	if err != nil || storageplugin == nil {
		//Not found in initialized plugins, so create a new one

		if configFilePath != "" {
			config, err := os.Open(configFilePath)
			if err != nil {
				/*glog.Fatalf("Couldn't open cloud provider configuration %s: %#v",
				configFilePath, err)*/
			}

			defer config.Close()
			storageplugin, err = GetPlugin(name, config)
		} else {
			// Pass explicit nil so plugins can actually check for nil. See
			// "Why is my nil error value not equal to nil?" in golang.org/doc/faq.
			storageplugin, err = GetPlugin(name, nil)
		}

		if err != nil {
			return nil, fmt.Errorf("could not init plugin %q: %v", name, err)
		}
		if storageplugin == nil {
			return nil, fmt.Errorf("unknown plugin %q", name)
		}
		//Add the plugin to the initialized plugins
		initPluginMutex.Lock()
		defer initPluginMutex.Unlock()
		initializedPlugins[name] = storageplugin
	}

	return storageplugin, nil
}
