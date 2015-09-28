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
	"github.com/codegangsta/negroni"
	"github.com/goincremental/negroni-sessions"
	"github.com/goincremental/negroni-sessions/cookiestore"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/natefinch/pie"
	"github.com/skyrings/skyring/authprovider"
	"github.com/skyrings/skyring/conf"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path"
)

const (
	// ConfigFile default configuration file
	ProviderConfDir   = "providers.d"
	ProviderBinaryDir = "providers"
)

type Provider struct {
	Name   string
	Client *rpc.Client
}

type App struct {
	providers    map[string]Provider
	routes       map[string]conf.Route
	authProvider authprovider.AuthInterface
}

type Args struct {
	Vars    map[string]string
	Request []byte
}

func NewApp(configDir string, binDir string) *App {
	app := &App{}

	//initialize the maps
	app.providers = make(map[string]Provider)
	app.routes = make(map[string]conf.Route)

	//Load providers and routes
	//Load all the files present in the config path
	app.StartProviders(configDir, binDir)

	//Check if atleast one provider is initialized successfully
	//otherwise panic
	if len(app.providers) < 1 {
		panic(fmt.Sprintf("None of the providers are initialized successfully"))
	}
	glog.Infof("Loaded URLs:", app.routes)

	return app
}

func (a *App) StartProviders(configDir string, binDir string) {

	providerBinaryPath := path.Join(binDir, ProviderBinaryDir)

	configs := conf.LoadProviderConfig(path.Join(configDir, ProviderConfDir))
	glog.Infof("Config:", configs)

	for _, config := range configs {
		glog.V(3).Infof("Config:", config)

		//check if the routes are unique
		//Load the routes later after the provider starts
		//successfully, otherwise we have to remove the routes if the provider
		//fails to start
		var found bool
		for _, element := range config.Routes {
			if _, ok := a.routes[element.Name]; ok {
				glog.Errorln("Error in Route configuration, Duplicate routes")
				//Dont proceed further
				found = true
				break
			}
		}
		//If duplicate routes are detected, dont proceed loading this provider
		if found {
			continue
		}
		//Start the provider if configured
		if config.Provider != (conf.ProviderConfig{}) {

			if config.Provider.Name == "" {
				continue
			}
			//check whether the plugin is initialized already
			if _, ok := a.providers[config.Provider.Name]; ok {
				//Provider already initialized
				continue
			}
			if config.Provider.ProviderBinary == "" {
				//set the default if not provided in config file
				config.Provider.ProviderBinary = path.Join(providerBinaryPath, config.Provider.Name)
			} else {
				config.Provider.ProviderBinary = path.Join(providerBinaryPath, config.Provider.ProviderBinary)
			}

			client, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, config.Provider.ProviderBinary)
			if err != nil {
				glog.Errorf("Error running plugin:%s", err)
				continue
			}
			//Load the routes
			for _, element := range config.Routes {
				a.routes[element.Name] = element
			}
			//add the provider to the map
			a.providers[config.Provider.Name] = Provider{Name: config.Provider.Name, Client: client}

		}

	}

}

func (a *App) SetRoutes(router *mux.Router) error {

	// Create a router for defining the routes which require authentication
	//For routes require auth, will be checked by a middleware which
	//return error immediately

	authReqdRouter := mux.NewRouter().StrictSlash(true)

	//Set the provider specific routes here
	//All the provider specific routes are assumed to be
	//authenticated

	for _, route := range a.routes {
		glog.V(3).Info(route)
		authReqdRouter.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(http.HandlerFunc(a.ProviderHandler))
	}
	//router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
	a.SetAuthRoutes(router, authReqdRouter)
	return nil
}

func (a *App) InitializeAuth(authCfg conf.AuthConfig, n *negroni.Negroni) error {

	//Load authorization middleware for session
	//TODO - make this plugin based, we should be able
	//to plug in based on the configuration - token, jwt token etc
	//Right we are supporting only session based auth
	store := cookiestore.New([]byte("SkyRing-secret"))
	n.Use(sessions.Sessions("skyring_session_store", store))

	//Initailize the backend auth provider based on the configuartion
	if aaa, err := authprovider.InitAuthProvider(authCfg.ProviderName, authCfg.ConfigFile); err != nil {
		glog.Errorf("Error Initializing the Authentication: %s", err)
		return err
	} else {
		a.authProvider = aaa
		return nil
	}
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
		glog.Errorf("Error parsing http request body: %s", err)
	}

	//Find out the provider to process this request and send the request
	//After getting the response, pass it on to the client
	provider := a.getProvider(body, routeCfg)
	if provider != nil {
		glog.Infof("Sending the request to provider:", provider.Name)
		provider.Client.Call(provider.Name+"."+routeCfg.PluginFunc, Args{Vars: vars, Request: body}, &result)
		//Parse the result to see if a different status needs to set
		//By default it sets http.StatusOK(200)
		glog.Infof("Got response from provider")
		var m map[string]interface{}
		if err = json.Unmarshal(result, &m); err != nil {
			glog.Errorf("Unable to Unmarshall the result from provider : %s", err)
		}
		status := m["Status"].(float64)
		if status != http.StatusOK {
			w.WriteHeader(int(status))
		}
		w.Write(result)
		return
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Matching Provider Not Found"))
	}
}

//Middleware to check the request is authenticated
func (a *App) LoginRequired(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	glog.Infof("Inside Auth check middleware")

	session := sessions.GetSession(r)
	sessionName := session.Get("username")

	if sessionName == nil {
		w.WriteHeader(http.StatusUnauthorized)
		glog.Infof("Not Authorized returning from here")
		return
	}
	glog.Infof("Inside Auth check middleware .. Fine Proceed")
	next(w, r)
}
