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
package app

import (
	"fmt"
	"github.com/emicklei/go-restful"
	"sync"
)

type Application interface {
	SetRoutes(container *restful.Container) error
}

/*
We have taken Kubernetes plugin architecture as a reference
https://github.com/kubernetes/kubernetes
*/

type AppFactory func(config string) (Application, error)

// All registered plugins
var appMutex sync.Mutex
var apps = make(map[string]AppFactory)

// RegisterPlugin registers a plugin by name.  This
// is expected to happen during app startup.
func RegisterApp(name string, factory AppFactory) {
	appMutex.Lock()
	defer appMutex.Unlock()
	if _, found := apps[name]; found {
		//glog.Fatalf("Cloud provider %q was registered twice", name)
	}
	//glog.V(1).Infof("Registered cloud provider %q", name)
	//var err error
	apps[name] = factory
}

func GetApp(name string, config string) (Application, error) {
	appMutex.Lock()
	defer appMutex.Unlock()
	f, found := apps[name]
	if !found {
		fmt.Println("App not found", name)
		return nil, nil
	}
	return f(config)
}

// InitPlugin creates an instance of the named plugin.
func InitApp(name string, configFilePath string) (Application, error) {
	var app Application
	fmt.Println("InitApp name", name)
	if name == "" {
		//glog.Info("No plugins specified.")
		return nil, nil
	}

	var err error

	if configFilePath != "" {
		app, err = GetApp(name, configFilePath)
	} else {
		// Pass explicit nil so plugins can actually check for nil. See
		// "Why is my nil error value not equal to nil?" in golang.org/doc/faq.
		app, err = GetApp(name, "")
	}

	if err != nil {
		return nil, fmt.Errorf("could not init plugin %q: %v", name, err)
	}
	if app == nil {
		return nil, fmt.Errorf("unknown plugin %q", name)
	}

	return app, nil
}
