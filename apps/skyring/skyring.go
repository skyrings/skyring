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
	"errors"
	"fmt"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/kidstuff/mongostore"
	"github.com/natefinch/pie"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/dbprovider"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/monitoring"
	"github.com/skyrings/skyring-common/tools/lock"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/schedule"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring/authprovider"
	"github.com/skyrings/skyring/nodemanager"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path"
	"time"
)

type Provider struct {
	Name   string
	Client *rpc.Client
}

type App struct {
	providers map[string]Provider
	routes    map[string]conf.Route
}

type ContextKey int

const (
	// ConfigFile default configuration file
	ProviderConfDir   = "providers.d"
	ProviderBinaryDir = "providers"
	//DefaultMaxAge set to a week
	DefaultMaxAge                 = 86400 * 7
	DEFAULT_API_PREFIX            = "/api"
	LoggingCtxt        ContextKey = 0
)

var (
	application          *App
	CoreNodeManager      nodemanager.NodeManagerInterface
	MonitoringManager    monitoring.MonitoringManagerInterface
	AuthProviderInstance authprovider.AuthInterface
	TaskManager          task.Manager
	Store                *mongostore.MongoStore
	DbManager            dbprovider.DbInterface
	lockManager          lock.LockManager
)

func NewApp(configDir string, binDir string) *App {
	app := &App{}

	//initialize the maps
	app.providers = make(map[string]Provider)
	app.routes = make(map[string]conf.Route)

	application = app
	return app
}

func (a *App) StartProviders(configDir string, binDir string) error {

	providerBinaryPath := path.Join(binDir, ProviderBinaryDir)

	configs := conf.LoadProviderConfig(path.Join(configDir, ProviderConfDir))
	logger.Get().Info("Config:", configs)

	for _, config := range configs {
		logger.Get().Debug("Config:", config)

		//check if the routes are unique
		//Load the routes later after the provider starts
		//successfully, otherwise we have to remove the routes if the provider
		//fails to start
		var found bool
		for _, element := range config.Routes {
			if _, ok := a.routes[element.Name]; ok {
				logger.Get().Error("Error in Route configuration, Duplicate route: %s", element.Name)
				//Dont proceed further
				found = true
				break
			}
		}
		//If duplicate routes are detected, dont proceed loading this provider
		if found {
			logger.Get().Error("Duplicate routes detected, skipping the provider", config)
			continue
		}
		//Start the provider if configured
		if config.Provider != (conf.ProviderConfig{}) {

			if config.Provider.Name == "" {
				logger.Get().Error("Provider Name Empty, skipping the provider", config)
				continue
			}
			//check whether the plugin is initialized already
			if _, ok := a.providers[config.Provider.Name]; ok {
				//Provider already initialized
				logger.Get().Info("Provider already initialized, skipping the provider", config)
				continue
			}
			if config.Provider.ProviderBinary == "" {
				//set the default if not provided in config file
				config.Provider.ProviderBinary = path.Join(providerBinaryPath, config.Provider.Name)
			} else {
				config.Provider.ProviderBinary = path.Join(providerBinaryPath, config.Provider.ProviderBinary)
			}

			confStr, _ := json.Marshal(conf.SystemConfig)
			client, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, config.Provider.ProviderBinary, string(confStr))
			if err != nil {
				logger.Get().Error("Error starting provider for %s. error: %v", config.Provider.Name, err)
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
	if len(a.providers) < 1 {
		return errors.New("None of the providers are initialized successfully")
	}
	return nil
}

func (a *App) SetRoutes(router *mux.Router) error {
	// Load App specific routes
	a.LoadRoutes()

	// Create a router for defining the routes which require authenticationAuth_fix
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
			logger.Get().Info("Skipped the route: %s as version: %d is un spported", route.Name, route.Version)
		}
	}

	// Set routes for core which require authentication
	for _, route := range CORE_ROUTES {
		if validApiVersion(route.Version) {
			urlPattern := fmt.Sprintf("%s/v%d/%s", DEFAULT_API_PREFIX, route.Version, route.Pattern)
			authReqdRouter.Methods(route.Method).Path(urlPattern).Name(route.Name).Handler(http.HandlerFunc(route.HandlerFunc))
			router.Handle(urlPattern, n)
		} else {
			logger.Get().Info("Skipped the route: %s as version: %d is un spported", route.Name, route.Version)
		}
	}

	//Set the provider specific routes here
	//All the provider specific routes are assumed to be authenticated
	for _, route := range a.routes {
		logger.Get().Debug("%s", route)
		if validApiVersion(route.Version) {
			urlPattern := fmt.Sprintf("%s/v%d/%s", DEFAULT_API_PREFIX, route.Version, route.Pattern)
			authReqdRouter.
				Methods(route.Method).
				Path(urlPattern).
				Name(route.Name).
				Handler(http.HandlerFunc(a.ProviderHandler))
			router.Handle(urlPattern, n)
		} else {
			logger.Get().Info("Skipped the route: %s as version: %d is un spported", route.Name, route.Version)
		}
	}

	return nil
}

func initializeAuth(authCfg conf.AuthConfig) error {
	//Load authorization middleware for session
	//TODO - make this plugin based, we should be able
	//to plug in based on the configuration - token, jwt token etc
	//Right we are supporting only session based auth

	session := db.GetDatastore().Copy()
	c := session.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_SESSION_STORE)
	Store = mongostore.NewMongoStore(c, DefaultMaxAge, true, []byte("SkyRing-secret"))

	//Initailize the backend auth provider based on the configuartion
	if aaa, err := authprovider.InitAuthProvider(authCfg.ProviderName, authCfg.ConfigFile); err != nil {

		logger.Get().Error("Error Initializing the Authentication Provider. error: %v", err)
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

func initializeDb(authCfg conf.AppDBConfig) error {

	if mgr, err := dbprovider.InitDbProvider("mongodbprovider", ""); err != nil {
		logger.Get().Error("Error Initializing the Db Provider: %s", err)
		return err
	} else {
		DbManager = mgr
	}

	if err := DbManager.InitDb(); err != nil {
		logger.Get().Error("Error Initializing the Collections: %s", err)
		return err
	}
	return nil
}

func GetDbProvider() dbprovider.DbInterface {
	return DbManager
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
		logger.Get().Error("Error parsing http request body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error parsing the http request body"))
	}

	//Find out the provider to process this request and send the request
	//After getting the response, pass it on to the client
	provider := a.getProviderFromRoute(routeCfg)
	if provider != nil {
		logger.Get().Info("Sending the request to provider: %s", provider.Name)
		provider.Client.Call(provider.Name+"."+routeCfg.PluginFunc, models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body}, &result)
		//Parse the result to see if a different status needs to set
		//By default it sets http.StatusOK(200)
		logger.Get().Info("Got response from provider: %s", provider.Name)
		var m models.RpcResponse
		if err = json.Unmarshal(result, &m); err != nil {
			logger.Get().Error("Unable to Unmarshall the result from provider: %s. error: %v", provider.Name, err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Unable to unmarshall the result from provider"))
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

func initializeNodeManager(config conf.NodeManagerConfig) error {
	if manager, err := nodemanager.InitNodeManager(config.ManagerName, config.ConfigFilePath); err != nil {
		logger.Get().Error("Error initializing the node manager. error: %v", err)
		return err
	} else {
		CoreNodeManager = manager
		return nil
	}
}

func (a *App) initializeMonitoringManager(config conf.MonitoringDBconfig) error {
	if manager, err := monitoring.InitMonitoringManager(config.ManagerName, config.ConfigFilePath); err != nil {
		logger.Get().Error("Error initializing the monitoring manager: %v", err)
		return err
	} else {
		MonitoringManager = manager
		return nil
	}
}

func scheduleSummaryMonitoring(config conf.SystemSummaryConfig) error {
	schedule.InitShechuleManager()
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		logger.Get().Error("Error scheduling the system summary calculation. Error %v", err)
		return err
	}
	f := Compute_System_Summary
	go scheduler.Schedule(time.Duration(config.NetSummaryInterval)*time.Second, f, make(map[string]interface{}))
	Compute_System_Summary(make(map[string]interface{}))
	return nil
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

func GetApp() *App {
	return application
}

func GetMonitoringManager() monitoring.MonitoringManagerInterface {
	if MonitoringManager == nil {
		logger.Get().Error("MonitoringManager is nil")
	}
	return MonitoringManager
}

//Middleware to check the request is authenticated
func (a *App) LoginRequired(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	session, err := Store.Get(r, "session-key")
	if err != nil {
		logger.Get().Error("Error Getting the session. error: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if session.IsNew {
		logger.Get().Info("Not Authorized")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	next(w, r)
}

//Middleware to create the logging context
func (a *App) LoggingContext(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	session, err := Store.Get(r, "session-key")
	if err != nil {
		logger.Get().Error("Error Getting the session. error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var username string
	if val, ok := session.Values["username"]; ok {
		username = val.(string)
	}

	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	loggingContext := fmt.Sprintf("%v:%v", username, reqId.String())

	context.Set(r, LoggingCtxt, loggingContext)

	defer context.Clear(r)
	next(w, r)
}

func initializeTaskManager() error {
	TaskManager = task.NewManager()
	return nil
}

func (a *App) GetTaskManager() *task.Manager {
	return &TaskManager
}

func initializeLockManager() error {
	lockManager = lock.NewLockManager()
	return nil
}

func (a *App) GetLockManager() lock.LockManager {
	return lockManager
}

/*
Initialize the defaults during app startup
*/
func initializeDefaults() error {
	if err := AddDefaultProfiles(); err != nil {
		logger.Get().Error("Default Storage profiles create failed: %v", err)
		return err
	}
	return nil
}

/*
Initialize the application
*/
func (a *App) InitializeApplication(sysConfig conf.SkyringCollection) error {

	if err := initializeNodeManager(sysConfig.NodeManagementConfig); err != nil {
		logger.Get().Error("Unable to create node manager")
		return err
	}
	/*
		TODO : This will be removed after porting all the existing things into newer scheme
	*/
	// Create DB session
	if err := db.InitDBSession(sysConfig.DBConfig); err != nil {
		logger.Get().Error("Unable to initialize DB")
		return err
	}
	if err := db.InitMonitoringDB(sysConfig.TimeSeriesDBConfig); err != nil {
		logger.Get().Error("Unable to initialize monitoring DB")
		return err
	}

	//Initialize the DB provider
	if err := initializeDb(sysConfig.DBConfig); err != nil {
		logger.Get().Error("Unable to initialize the authentication provider: %s", err)
		return err
	}

	//Initialize the auth provider
	if err := initializeAuth(sysConfig.Authentication); err != nil {
		logger.Get().Error("Unable to initialize the authentication provider: %s", err)
		return err
	}

	//Initialize the task manager
	if err := initializeTaskManager(); err != nil {
		logger.Get().Error("Unable to initialize the task manager: %s", err)
		return err
	}

	//Initialize the lock manager
	if err := initializeLockManager(); err != nil {
		logger.Get().Error("Unable to initialize the lock manager: %s", err)
		return err
	}

	//Initialize the Defaults
	if err := initializeDefaults(); err != nil {
		logger.Get().Error("Unable to initialize the Defaults: %s", err)
		return err
	}

	if err := application.initializeMonitoringManager(sysConfig.TimeSeriesDBConfig); err != nil {
		logger.Get().Error("Unable to create monitoring manager")
		os.Exit(1)
	}

	return nil
}

func (a *App) PostInitApplication(sysConfig conf.SkyringCollection) error {

	logger.Get().Info("Starting clusters syncing")
	go a.SyncClusterDetails()
	go InitSchedules()

	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return fmt.Errorf("Error Creating the RequestId. error: %v", err)
	}

	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())

	if err := scheduleSummaryMonitoring(sysConfig.SummaryConfig); err != nil {
		logger.Get().Error("%s - Failed to schedule fetching summary.Error %v", ctxt, err)
		return err
	}
	schedule_task_check()
	return nil
}

func schedule_task_check() {
	//Check Task status
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		logger.Get().Error(err.Error())
	} else {
		f := check_task_status
		m := make(map[string]interface{})
		go scheduler.Schedule(time.Duration(10)*time.Minute, f, m)
	}
}

func check_task_status(params map[string]interface{}) {
	var defaultTime, t time.Time
	var id uuid.UUID
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var Tasks []models.AppTask
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	if err := collection.Find(bson.M{"completed": false, "parentid": id}).All(&Tasks); err != nil && err != mgo.ErrNotFound {
		logger.Get().Warning(err.Error())
		return
	}
	for _, Task := range Tasks {
		t = Task.LastUpdated
		if t == defaultTime {
			t = Task.StatusList[len(Task.StatusList)-1].Timestamp
		}
		Duration := time.Since(t)
		minutes := int(Duration.Minutes())
		if minutes >= 10 {
			// Fetch the child tasks for this task id
			var ChildTasks []models.AppTask
			if err := collection.Find(bson.M{"completed": false, "parentid": Task.Id}).All(&ChildTasks); err != nil && err != mgo.ErrNotFound {
				logger.Get().Warning(err.Error())
				return
			}
			for _, ChildTask := range ChildTasks {
				var req models.RpcRequest
				var result models.RpcResponse
				app := GetApp()
				provider := app.getProviderFromClusterType(ChildTask.Owner)
				vars := make(map[string]string)
				vars["task-id"] = ChildTask.Id.String()
				err := provider.Client.Call(fmt.Sprintf("%s.%s",
					provider.Name, "StopTask"),
					models.RpcRequest{RpcRequestVars: vars, RpcRequestData: req.RpcRequestData},
					&result)
				if err != nil || (result.Status.StatusCode != http.StatusOK) {
					logger.Get().Warning(fmt.Sprintf("Error in stopping  child task: %v.error:%v", ChildTask.Id, err))
					return
				}
			}
			//Stoping parent task
			if ok, _ := TaskManager.Stop(Task.Id); !ok {
				logger.Get().Error("Failed to stop task: %v", Task.Id)
			}
		}
	}
}
