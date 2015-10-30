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

package authprovider

import (
	"fmt"
	"github.com/skyrings/skyring/tools/logger"
	"io"
	"os"
	"sync"
)

type Plugin struct {
	Name       string
	AuthPlugin AuthInterface
}

/*
We have taken Kubernetes plugin architecture as a reference
https://github.com/kubernetes/kubernetes
*/

type ProvidersFactory func(config io.Reader) (AuthInterface, error)

// All registered providers
var providersMutex sync.Mutex
var providers = make(map[string]ProvidersFactory)

// RegisterPlugin registers a plugin by name.  This
// is expected to happen during app startup.
func RegisterAuthProvider(name string, factory ProvidersFactory) {
	providersMutex.Lock()
	defer providersMutex.Unlock()
	if _, found := providers[name]; found {
		logger.Get().Critical("Auth provider %q was registered twice", name)
	}
	providers[name] = factory
}

func GetAuthProvider(name string, config io.Reader) (AuthInterface, error) {
	providersMutex.Lock()
	defer providersMutex.Unlock()
	f, found := providers[name]
	if !found {
		return nil, nil
	}
	return f(config)
}

// InitPlugin creates an instance of the named plugin.
func InitAuthProvider(name string, configFilePath string) (AuthInterface, error) {
	var authprovider AuthInterface

	if name == "" {
		logger.Get().Info("No providers specified.")
		return nil, nil
	}

	var err error

	if configFilePath != "" {
		config, err := os.Open(configFilePath)
		if err != nil {
			logger.Get().Critical("Couldn't open auth provider configuration %s: %#v",
				configFilePath, err)
		}

		defer config.Close()
		authprovider, err = GetAuthProvider(name, config)
	} else {
		// Pass explicit nil so providers can actually check for nil. See
		// "Why is my nil error value not equal to nil?" in golang.org/doc/faq.
		authprovider, err = GetAuthProvider(name, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("could not init plugin %q: %v", name, err)
	}
	if authprovider == nil {
		return nil, fmt.Errorf("unknown plugin %q", name)
	}

	return authprovider, nil
}
