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
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/natefinch/pie"
	"github.com/skyrings/skyring/conf"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"sync"
)

type Provider struct {
	Name   string
	Client *rpc.Client
}

type App struct {
	providers map[string]Provider
	urls      map[string]conf.Route
}

type Args struct {
	Vars    map[string]string
	Request []byte
}

func NewApp(configfile string) *App {
	app := &App{}

	app.providers = make(map[string]Provider)

	//Load the plugins from the config file

	pluginCollection := conf.LoadPluginConfiguration(configfile)

	for _, element := range pluginCollection.Plugins {
		client, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, element.PluginBinary)
		if err != nil {
			glog.Errorf("Error running plugin: %s", err)
		}

		app.providers[element.Name] = Provider{Name: element.Name, Client: client}

	}

	//Load URLs
	app.urls = make(map[string]conf.Route)
	urls := conf.LoadUrls(pluginCollection.UrlConfigPath)
	for _, element := range urls.Routes {
		app.urls[element.Name] = element
	}
	glog.Infof("Loaded URLs:", app.urls)
	return app
}

func (a *App) SetRoutes(router *mux.Router) error {
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
	for _, route := range a.urls {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(http.HandlerFunc(a.ProviderHandler))
	}
	return nil
}

func (a *App) ProviderHandler(w http.ResponseWriter, r *http.Request) {

	//Parse the Request and get the parameters and route information
	route := mux.CurrentRoute(r)
	vars := mux.Vars(r)

	var result []byte

	//Get the route details from the map
	routeCfg := a.urls[route.GetName()]

	//Get the request details from requestbody
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		//log the error
	}

	//Broadcast the request to all the registered providers. The provider will take call to process it or not
	var wg sync.WaitGroup
	for _, provider := range a.providers {
		wg.Add(1)
		go func(provider Provider) {
			defer wg.Done()
			provider.Client.Call(provider.Name+"."+routeCfg.PluginFunc, Args{Vars: vars, Request: body}, &result)
			var m map[string]interface{}
			json.Unmarshal(result, &m)
			//The providers return {Status: "Not Supported"} if recieves invalid request. Ignore those and process only
			//the valid response
			if m["Status"] != "Not Supported" {
				w.Write(result)
			}
		}(provider)
	}
	wg.Wait()
}
