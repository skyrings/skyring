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
	"io/ioutil"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"skyring/auth"
	"skyring/conf"
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
			glog.Errorf("Error running plugin:", err)
		}

		app.providers[element.Name] = Provider{Name: element.Name, Client: client}

	}

	//Load URLs
	//Load all the files present in the URL config path
	app.urls = make(map[string]conf.Route)
	files, _ := ioutil.ReadDir(pluginCollection.UrlConfigPath)
	for _, f := range files {
		glog.Infof("File Name:", f.Name())
		urls := conf.LoadUrls(pluginCollection.UrlConfigPath + "/" + f.Name())
		for _, element := range urls.Routes {
			app.urls[element.Name] = element
		}
	}

	glog.Infof("Loaded URLs:", app.urls)

	//Init the Auth
	auth.InitAuthorizer()
	return app
}

func (a *App) SetRoutes(container *mux.Router) error {
	//container.HandleFunc("/", a.ProviderHandler)
	for _, route := range a.urls {
		container.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(http.HandlerFunc(a.ProviderHandler))
	}
	//Add the Skyring Core specific routes
	auth.SetAuthRoutes(container)
	return nil
}

/*
This is the handler where all the requests to providers will land in.
Here the parameters are extracted and paaed to the providers along with
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
	routeCfg := a.urls[route.GetName()]

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
		var m map[string]interface{}
		if err = json.Unmarshal(result, &m); err != nil {
			glog.Errorf("Unable to Unmarshall the result from provider", err)
		}
		if m["Status"] != http.StatusOK {
			w.WriteHeader(m["Status"].(int))
		}
		w.Write(result)
		return
	}
}
