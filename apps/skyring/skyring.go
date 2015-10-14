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
	"github.com/op/go-logging"
	"github.com/gorilla/mux"
	"github.com/natefinch/pie"
	"github.com/skyrings/skyring/authprovider"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/nodemanager"
	"github.com/skyrings/skyring/tools/logger"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path"
)

log := logger.Get()

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
	providers map[string]Provider
	routes    map[string]conf.Route
}

const (
	DEFAULT_API_PREFIX = "/api"
)

var (
	CoreNodeManager      nodemanager.NodeManagerInterface
	AuthProviderInstance authprovider.AuthInterface
)

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
	/*if len(app.providers) < 1 {
		panic(fmt.Sprintf("None of the providers are initialized successfully"))
	}
	log.Info("Loaded URLs:", app.routes)*/

	return app
}

func (a *App) StartProviders(configDir string, binDir string) {

	providerBinaryPath := path.Join(binDir, ProviderBinaryDir)

	configs := conf.LoadProviderConfig(path.Join(configDir, ProviderConfDir))
	log.Info("Config:", configs)

	for _, config := range configs {
		log.Debug("Config:", config)

		//check if the routes are unique
		//Load the routes later after the provider starts
		//successfully, otherwise we have to remove the routes if the provider
		//fails to start
		var found bool
		for _, element := range config.Routes {
			if _, ok := a.routes[element.Name]; ok {
				log.Error("Error in Route configuration, Duplicate routes")
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

			dbConfStr, _ := json.Marshal(conf.SystemConfig.DBConfig)
			client, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, config.Provider.ProviderBinary, string(dbConfStr))
			if err != nil {
				log.Error("Error running plugin:%s", err)
				continue
			}
			//Load the routes
			for _, element := range config.Routes {
				//prefix the provider name to the route
				element.Pattern = fmt.Sprintf("%s/%s", config.Provider.Name, element.Pattern)
				a.routes[element.Name] = element
			}
			//add the provider to the map
			a.providers[config.Provider.Name] = Provider{Name: config.Provider.Name, Client: client}
		}
	}
}

func (a *App) SetRoutes(router *mux.Router) error {
	// Load App specific routes
	a.LoadRoutes()

	// Create a router for defining the routes which require authentication
	//For routes require auth, will be checked by a middleware which
	//return error immediately

	authReqdRouter := mux.NewRouter().StrictSlash(true)

	//create a negroni with LoginReqd middleware
	n := negroni.New(negroni.HandlerFunc(a.LoginRequired), negroni.Wrap(authReqdRouter))

	// Set routes for core which doesnot require authentication
	for _, route := range CORE_ROUTES_NOAUTH {
		if validApiVersion(route.Version) {
			urlPattern := fmt.Sprintf("%s/v%d/%s", DEFAULT_API_PREFIX, route.Version, route.Pattern)
			router.Methods(route.Method).Path(urlPattern).Name(route.Name).Handler(http.HandlerFunc(route.HandlerFunc))
		} else {
			log.Info("Skipped the route: %s as version is un spported", route.Name)
		}
	}

	// Set routes for core which require authentication
	for _, route := range CORE_ROUTES {
		if validApiVersion(route.Version) {
			urlPattern := fmt.Sprintf("%s/v%d/%s", DEFAULT_API_PREFIX, route.Version, route.Pattern)
			authReqdRouter.Methods(route.Method).Path(urlPattern).Name(route.Name).Handler(http.HandlerFunc(route.HandlerFunc))
			router.Handle(urlPattern, n)
		} else {
			log.Info("Skipped the route: %s as version is un spported", route.Name)
		}
	}

	//Set the provider specific routes here
	//All the provider specific routes are assumed to be authenticated
	for _, route := range a.routes {
		log.Debug(route)
		if validApiVersion(route.Version) {
			urlPattern := fmt.Sprintf("%s/v%d/%s", DEFAULT_API_PREFIX, route.Version, route.Pattern)
			authReqdRouter.
				Methods(route.Method).
				Path(urlPattern).
				Name(route.Name).
				Handler(http.HandlerFunc(a.ProviderHandler))
			router.Handle(urlPattern, n)
		} else {
			log.Info("Skipped the route: %s as version is un spported", route.Name)
		}
	}

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
		log.Error("Error Initializing the Authentication: %s", err)
		return err
	} else {
		AuthProviderInstance = aaa
	}
	AddDefaultUser()
	return nil
}

func GetAuthProvider() authprovider.AuthInterface {
	return AuthProviderInstance
}

/*
This is the handler where all the requests to providers will land in.
Here the parameters are extracted and passed to the providers along with
the requestbody if any using RPC. Result will be parsed to see if a specific
status code needs to be set for http response.
Arguments to Provider function -
1. type RpcRequest struct {
	RpcRequestVars map[string]string
	RpcRequestData []byte
}
Each provider should expect this structure as the first argument. RpcRequestVars
is a map of parameters passed in request URL. RpcRequestData is the payload(body)
of the http request.

2.Result - *[]byte
Pointer to a byte array. In response, the byte array should contain
{
    "status": {"statuscode": <Code>, "statusmessage": "<msg>"},
    "data": {"requestid": "<id>", "result": []byte}
}
where result is response payload from the provider, RequestId is populated if the
request is asynchronously executed and statuscode and statusmessage to be set in the
status field. The statuscode field should be valid http status code
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
		log.Error("Error parsing http request body: %s", err)
	}

	//Find out the provider to process this request and send the request
	//After getting the response, pass it on to the client
	provider := a.getProviderFromRoute(routeCfg)
	if provider != nil {
		log.Info("Sending the request to provider:", provider.Name)
		provider.Client.Call(provider.Name+"."+routeCfg.PluginFunc, models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body}, &result)
		//Parse the result to see if a different status needs to set
		//By default it sets http.StatusOK(200)
		log.Info("Got response from provider")
		var m models.RpcResponse
		if err = json.Unmarshal(result, &m); err != nil {
			log.Error("Unable to Unmarshall the result from provider : %s", err)
		}
		status := m.Status.StatusCode
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

func (a *App) InitializeNodeManager(config conf.NodeManagerConfig) error {
	if manager, err := nodemanager.InitNodeManager(config.ManagerName, config.ConfigFilePath); err != nil {
		log.Error("Error initializing the node manager: %v", err)
		return err
	} else {
		CoreNodeManager = manager
		return nil
	}
}

func validApiVersion(version int) bool {
	for _, ver := range conf.SystemConfig.Config.SupportedVersions {
		if ver == version {
			return true
		}
	}
	return false
}

func GetCoreNodeManager() nodemanager.NodeManagerInterface {
	return CoreNodeManager
}

//Middleware to check the request is authenticated
func (a *App) LoginRequired(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {

	session := sessions.GetSession(r)
	sessionName := session.Get("username")

	if sessionName == nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Info("Not Authorized returning from here")
		return
	}
	next(w, r)
}
