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
	"github.com/skyrings/skyring-common/provisioner"
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
	"strings"
	"time"
)

type Provider struct {
	Name            string
	Client          *rpc.Client
	ProvisionerName provisioner.Provisioner
}

type App struct {
	providers map[string]Provider
	routes    map[string]conf.Route
}

type ContextKey int

const (
	//DefaultMaxAge set to a week
	DefaultMaxAge                 = 86400 * 7
	DEFAULT_API_PREFIX            = "/api"
	LoggingCtxt        ContextKey = 0
	Timeout                       = 10
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
	EventTypes           map[string]string
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

	providerBinaryPath := path.Join(binDir, conf.ProviderBinaryDir)

	configs := conf.LoadProviderConfig(path.Join(configDir, conf.ProviderConfDir))
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
			//Initilaize the provisioner
			prov, err := provisioner.InitializeProvisioner(config.Provisioner)
			if err != nil {
				logger.Get().Error("Unable to initialize the provisioner, skipping the provider:%v", config)
				continue
			}
			if config.Provider.ProviderBinary == "" {
				//set the default if not provided in config file
				config.Provider.ProviderBinary = path.Join(providerBinaryPath, config.Provider.Name)
			} else {
				config.Provider.ProviderBinary = path.Join(providerBinaryPath, config.Provider.ProviderBinary)
			}

			confStr, _ := json.Marshal(conf.SystemConfig)
			eventTypesStr, _ := json.Marshal(EventTypes)
			client, err := pie.StartProviderCodec(
				jsonrpc.NewClientCodec,
				os.Stderr,
				config.Provider.ProviderBinary,
				string(confStr),
				string(eventTypesStr))
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
			a.providers[config.Provider.Name] = Provider{Name: config.Provider.Name, Client: client, ProvisionerName: prov}
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
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	//Parse the Request and get the parameters and route information
	route := mux.CurrentRoute(r)
	vars := mux.Vars(r)

	var result []byte

	//Get the route details from the map
	routeCfg := a.routes[route.GetName()]
	//Get the request details from requestbody
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body: %v", ctxt, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error parsing the http request body"))
	}

	//Find out the provider to process this request and send the request
	//After getting the response, pass it on to the client
	provider := a.getProviderFromRoute(ctxt, routeCfg)
	if provider != nil {
		logger.Get().Info("%s-Sending the request to provider: %s", ctxt, provider.Name)
		provider.Client.Call(provider.Name+"."+routeCfg.PluginFunc, models.RpcRequest{RpcRequestVars: vars, RpcRequestData: body}, &result)
		//Parse the result to see if a different status needs to set
		//By default it sets http.StatusOK(200)
		logger.Get().Info("Got response from provider: %s", provider.Name)
		var m models.RpcResponse
		if err = json.Unmarshal(result, &m); err != nil {
			logger.Get().Error("%s-Unable to Unmarshall the result from provider: %s. error: %v", ctxt, provider.Name, err)
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
	f := ComputeSystemSummary
	go scheduler.Schedule(time.Duration(config.NetSummaryInterval)*time.Second, f, make(map[string]interface{}))
	ComputeSystemSummary(make(map[string]interface{}))
	return nil
}

func schedulePhysicalResourceStatsFetch(params map[string]interface{}) error {
	schedule.InitShechuleManager()
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		logger.Get().Error("Error scheduling the node resource fetch. Error %v", err)
		return err
	}
	f := SyncNodeUtilizations
	go scheduler.Schedule(time.Duration(5)*time.Second, f, params)
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
func (a *App) InitializeApplication(sysConfig conf.SkyringCollection, configDir string) error {

	// Load the event type details
	if err := application.LoadEventTypes(
		configDir,
		fmt.Sprintf("%s.evt", models.ENGINE_NAME)); err != nil {
		logger.Get().Error("Failed to initialize event types. error: %v",
			err)
		return err
	}

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
	// Initialize the scheduler
	schedule.InitShechuleManager()

	// Create syncing schedule
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		logger.Get().Error("Error scheduling clusters syncing")
	} else {
		if sysConfig.ScheduleConfig.ClustersSyncInterval == 0 {
			sysConfig.ScheduleConfig.ClustersSyncInterval = 86400 // 24hrs
		}
		go scheduler.Schedule(
			time.Duration(sysConfig.ScheduleConfig.ClustersSyncInterval)*time.Second,
			a.SyncClusterDetails,
			nil)
	}

	// Create monitoring schedule
	go InitMonitoringSchedules()

	// First time sync of the cluster details while startup
	go a.SyncClusterDetails(nil)

	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return fmt.Errorf("Error Creating the RequestId. error: %v", err)
	}

	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())
	if err := schedulePhysicalResourceStatsFetch(map[string]interface{}{"ctxt": ctxt}); err != nil {
		logger.Get().Error("%s - Failed to schedule fetching node resource utilizations.Error %v", ctxt, err)
		return err
	}

	if err := scheduleSummaryMonitoring(sysConfig.SummaryConfig); err != nil {
		logger.Get().Error("%s - Failed to schedule fetching summary.Error %v", ctxt, err)
		return err
	}
	schedule_task_check(ctxt)
	node_Reinitialize()
	cleanupTasks()
	initializeAbout(ctxt)
	schedule_archive_activities(ctxt)
	return nil
}

func schedule_task_check(ctxt string) {
	//Check Task status
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		logger.Get().Error("%s-%v", ctxt, err.Error())
	} else {
		f := check_task_status
		m := make(map[string]interface{})
		go scheduler.Schedule(time.Duration(20*time.Minute), f, m)
	}
}

func check_task_status(params map[string]interface{}) {
	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return
	}
	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())
	var id uuid.UUID
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var tasks []models.AppTask
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	if err := collection.Find(bson.M{"completed": false, "parentid": id}).All(&tasks); err != nil {
		logger.Get().Warning("%s-%v", ctxt, err.Error())
		return
	}
	application := GetApp()
	for _, task := range tasks {
		if ok := check_TaskLastUpdate(task); ok {
			// Fetch the child tasks for this task id
			var subTasks []models.AppTask
			if err := collection.Find(bson.M{"completed": false, "parentid": task.Id}).All(&subTasks); err != nil && err != mgo.ErrNotFound {
				logger.Get().Warning("%s-%v", ctxt, err.Error())
				return
			}
			if len(subTasks) == 0 {
				//Stopping parent task
				stop_ParentTask(task, ctxt)
			} else {
				var result models.RpcResponse
				var stoppedSubTasksCount = 0
				for _, subTask := range subTasks {
					if ok := check_TaskLastUpdate(subTask); ok {
						provider := application.getProviderFromClusterType(subTask.Owner)
						vars := make(map[string]string)
						vars["task-id"] = subTask.Id.String()
						err := provider.Client.Call(fmt.Sprintf("%s.%s",
							provider.Name, "StopTask"),
							models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
							&result)
						if err != nil || (result.Status.StatusCode != http.StatusOK) {
							logger.Get().Warning(fmt.Sprintf(":%s-Error stopping sub task: %v. error:%v", ctxt, subTask.Id, err))
							continue
						}
						stoppedSubTasksCount++
					}
				}
				if stoppedSubTasksCount == len(subTasks) {
					// Stopping parent task
					stop_ParentTask(task, ctxt)
				}
			}
		}
	}
}

func check_TaskLastUpdate(task models.AppTask) bool {
	var defaultTime time.Time
	lastUpdated := task.LastUpdated
	if lastUpdated == defaultTime && len(task.StatusList) != 0 {
		lastUpdated = task.StatusList[len(task.StatusList)-1].Timestamp
	}
	duration := int(time.Since(lastUpdated).Minutes())
	if duration >= Timeout {
		return true
	}
	return false
}

func stop_ParentTask(task models.AppTask, ctxt string) {
	// Stoping task
	if ok, err := application.GetTaskManager().Stop(task.Id); !ok || err != nil {
		logger.Get().Error("%s-Failed to stop task: %v", ctxt, task.Id)
	}
}

func node_Reinitialize() {
	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return
	}
	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())
	var nodes models.Nodes
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := collection.Find(bson.M{"state": models.NODE_STATE_INITIALIZING}).All(&nodes); err != nil {
		logger.Get().Debug("%s-%v", ctxt, err.Error())
		return
	}
	for _, node := range nodes {
		go start_Reinitialize(node.Hostname, ctxt)

	}
}

func start_Reinitialize(hostname string, ctxt string) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if ok, err := GetCoreNodeManager().IsNodeUp(hostname, ctxt); !ok {
		logger.Get().Error(fmt.Sprintf("%s-Error getting status of node: %s. error: %v", ctxt, hostname, err))
		if err := collection.Update(bson.M{"hostname": hostname},
			bson.M{"$set": bson.M{"state": models.NODE_STATE_FAILED}}); err != nil {
			logger.Get().Critical("%s-Error Updating the node: %s. error: %v", ctxt, hostname, err)
		}
		return
	}
	Initialize(hostname, ctxt)
}

func cleanupTasks() {
	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return
	}
	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	s := []models.Status{{time.Now(), "Force Stop. Task explicitly stopped."}}
	if _, err := collection.UpdateAll(bson.M{"completed": false}, bson.M{"$set": bson.M{"completed": true,
		"status": models.TASK_STATUS_FAILURE, "statuslist": s}}); err != nil {
		logger.Get().Debug("%s-%v", ctxt, err.Error())
	}
}

func initializeAbout(ctxt string) {
	if len(conf.SystemConfig.SysCapabilities.StorageProviderDetails) != 0 {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_SYSTEM_CAPABILITIES)
		if _, err := coll.Upsert(bson.M{"productname": conf.SystemConfig.SysCapabilities.ProductName}, conf.SystemConfig.SysCapabilities); err != nil {
			logger.Get().Error(fmt.Sprintf("%s-Error adding System_capabilities details . error: %v", ctxt, err))
		}
	}
}

func (a *App) LoadEventTypes(configDir string, configFile string) error {
	var coreEventTypes = make(map[string]string)
	var providerEventTypes = make(map[string]string)
	file, err := ioutil.ReadFile(path.Join(configDir, configFile))
	if err != nil {
		return fmt.Errorf(
			"Error reading event types from %s",
			path.Join(configDir, configFile))
	}
	err = json.Unmarshal(file, &coreEventTypes)
	if err != nil {
		return fmt.Errorf(
			"Error un-marshalling core event types. error: %v",
			err)
	}

	files, err := ioutil.ReadDir(path.Join(configDir, conf.ProviderConfDir))
	if err != nil {
		return fmt.Errorf(
			"Error reading provider specific events. error: %v",
			err)
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".evt") {
			file, err := ioutil.ReadFile(path.Join(
				configDir,
				conf.ProviderConfDir,
				f.Name()))
			if err != nil {
				logger.Get().Critical(
					"Error reading events list from %s",
					f.Name())
				continue
			}
			err = json.Unmarshal(file, &providerEventTypes)
			if err != nil {
				logger.Get().Critical(
					"Error unmarshalling event types from %s",
					f.Name())
				continue
			}
			for key, value := range providerEventTypes {
				if _, ok := coreEventTypes[key]; ok {
					return fmt.Errorf(
						"Duplicate event type: %s found",
						key)
				}
				coreEventTypes[key] = value
			}
		}
	}
	EventTypes = coreEventTypes
	return nil
}

func schedule_archive_activities(ctxt string) {
	scheduler, err := schedule.NewScheduler()
	if err != nil {
		logger.Get().Error("%s-%v", ctxt, err.Error())
	} else {
		f := archive_activities
		m := make(map[string]interface{})
		m["ctxt"] = ctxt
		go scheduler.Schedule(time.Duration(24*time.Hour), f, m)
	}
}

func archive_activities(params map[string]interface{}) {
	ctxt := params["ctxt"].(string)
	archive_tasks(ctxt)
	archive_events(ctxt)
}

func archive_tasks(ctxt string) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var tasks []models.AppTask
	t := time.Now()
	t = t.AddDate(0, -1, 0)
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	if err := collection.Find(bson.M{"completed": true, "lastupdated": bson.M{"$lte": t}}).All(&tasks); err != nil {
		logger.Get().Debug("%s-%v", ctxt, err.Error())
		return
	}
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_ARCHIVE_TASKS)
	for _, task := range tasks {
		if err := coll.Insert(task); err != nil {
			logger.Get().Debug(fmt.Sprintf("%s-Error adding Archive task detail :%v .error : %v", ctxt, task.Id, err))
			continue
		}
		if err := collection.Remove(bson.M{"id": task.Id}); err != nil {
			logger.Get().Debug(fmt.Sprintf("%s-Error removing task detail :%v .error: %v", ctxt, task.Id, err))
			continue
		}
	}
}

func archive_events(ctxt string) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var events []models.AppEvent
	t := time.Now()
	t = t.AddDate(0, -1, 0)
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_APP_EVENTS)
	if err := collection.Find(bson.M{"$or": []interface{}{bson.M{"severity": models.ALARM_STATUS_CLEARED},
		bson.M{"systemackcomment": bson.M{"$ne": ""}}}, "timestamp": bson.M{"$lte": t}}).All(&events); err != nil {
		logger.Get().Debug("%s-%v", ctxt, err.Error())
		return
	}
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_ARCHIVE_EVENTS)
	for _, event := range events {
		if err := coll.Insert(event); err != nil {
			logger.Get().Debug(fmt.Sprintf("%s-Error adding Archive event detail :%v .error : %v", ctxt, event.EventId, err))
			continue
		}
		if err := collection.Remove(bson.M{"eventid": event.EventId}); err != nil {
			logger.Get().Debug(fmt.Sprintf("%s-Error removing event detail :%v .error: %v", ctxt, event.EventId, err))
			continue
		}
	}
}
