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
package skyring

import (
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/natefinch/pie"
	"github.com/skyrings/skyring/conf"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
)

type Provider struct {
	Name   string
	Client *rpc.Client
}

type App struct {
	providers map[string]Provider
	routes    map[string]conf.Route
}

type Args struct {
	Vars    map[string]string
	Request []byte
}

func NewApp(configfilePath string) *App {
	app := &App{}

	//initialize the maps
	app.providers = make(map[string]Provider)
	app.routes = make(map[string]conf.Route)

	//Load providers and routes
	//Load all the files present in the config path
	app.InitializeProviders(configfilePath)

	//Check if atleast one provider is initialized successfully
	//otherwise panic
	if len(app.providers) < 1 {
		panic(fmt.Sprintf("None of the providers are initialized successfully"))
	}
	glog.Infof("Loaded URLs:", app.routes)

	return app
}

func (a *App) InitializeProviders(configfilePath string) {
	if configfilePath == "" {
		//set to the default path
		configfilePath = "/etc/skyring/providers"
	}

	files, err := ioutil.ReadDir(configfilePath)
	if err != nil {
		glog.Errorf("Unable Read Config files", err)
		glog.Errorf("Failed to Initialize")
		panic(fmt.Sprintf("Unable Read Config files", err))
	}
	for _, f := range files {
		glog.Infof("File Name:", f.Name())
		config := conf.LoadProviderConfig(configfilePath + "/" + f.Name())
		for _, element := range config.Routes {
			a.routes[element.Name] = element
		}

		//Start the provider if configured
		if config.Plugin != (conf.PluginConfig{}) {

			if config.Plugin.Name == "" {
				continue
			}
			//check whether the plugin is initialized already
			if _, ok := a.providers[config.Plugin.Name]; ok {
				//Provider already initialized
				continue
			}
			if config.Plugin.PluginBinary == "" {
				//set the default if not provided in config file
				config.Plugin.PluginBinary = " /var/lib/skyring/providers/" + config.Plugin.Name
			}
			client, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, config.Plugin.PluginBinary)
			if err != nil {
				glog.Errorf("Error running plugin:", err)
				continue
			}

			a.providers[config.Plugin.Name] = Provider{Name: config.Plugin.Name, Client: client}

		}

	}

}

func (a *App) SetRoutes(router *mux.Router) error {
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
	for _, route := range a.routes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(http.HandlerFunc(a.ProviderHandler))
	}
	return nil
}

/*
This is the handler where all the requests to providers will land in.
Here the parameters are extracted and passed to the providers along with
the requestbody if any using RPC.Result will be parsed to see if a specific
status code needs to be set for http response.
Arguments to Provider function -
1. type Args struct {
	Vars    map[string]string
	Request []byte
}
Each provider should expect this structure as the first argument. Vars is a map
of parameters passed in request URL. Request is the payload(body) of the http request.

2.Result - *[]byte
Pointer to a byte array. In response, the byte array should contain
{ response payload, RequestId, Status} where response payload is response
from the provider, RequestId is populated if the request is asynchronously
executed and status to set in the http response
*/
func (a *App) ProviderHandler(w http.ResponseWriter, r *http.Request) {

	//Parse the Request and get the parameters and route information
	route := mux.CurrentRoute(r)
	vars := mux.Vars(r)

	var result []byte

	//Get the route details from the map
	routeCfg := a.routes[route.GetName()]

	//Get the request details from requestbody
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		glog.Errorf("Error parsing http request body", err)
	}

	//Find out the provider to process this request and send the request
	//After getting the response, pass it on to the client
	provider := a.getProvider(body, routeCfg)
	glog.Infof("Sending the request to provider:", provider.Name)
	if provider != nil {
		provider.Client.Call(provider.Name+"."+routeCfg.PluginFunc, Args{Vars: vars, Request: body}, &result)
		//Parse the result to see if a different status needs to set
		//By default it sets http.StatusOK(200)
		glog.Infof("Got response from provider")
		var m map[string]interface{}
		if err = json.Unmarshal(result, &m); err != nil {
			glog.Errorf("Unable to Unmarshall the result from provider", err)
		}
		status := m["Status"].(float64)
		if status != http.StatusOK {
			w.WriteHeader(int(status))
		}
		w.Write(result)
		return
	}
}
