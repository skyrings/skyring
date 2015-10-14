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

package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/mux"
	"github.com/op/go-logging"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/event"
	"github.com/skyrings/skyring/tools/logger"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

const (
	// ConfigFile default configuration file
	ConfigFile = "skyring.conf"
	// DefaultLogLevel default log level
	DefaultLogLevel = logging.DEBUG
)

var (
	configDir    string
	logFile      string
	logToStderr  bool
	logLevel     string
	providersDir string
	eventSocket  string
)

func main() {
	app := cli.NewApp()
	app.EnableBashCompletion = true
	app.Name = "skyring"
	app.Version = VERSION + "-" + RELEASE
	app.Authors = []cli.Author{{Name: "SkyRing", Email: "skyring@redhat.com"}}
	app.Usage = "Unified storage management service"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "config-dir, d",
			Value:  "/etc/skyring",
			Usage:  "Override default configuration directory",
			EnvVar: "SKYRING_CONFIGDIR",
		},
		cli.StringFlag{
			Name:   "providers-dir, b",
			Value:  "/var/lib/skyring",
			Usage:  "Override default bin directory",
			EnvVar: "SKYRING_PROVIDERSDIR",
		},
		cli.StringFlag{
			Name:  "event-socket",
			Value: "/tmp/.skyring-event",
			Usage: "Override default event unix socket",
		},
		cli.BoolFlag{
			Name:  "log-to-stderr",
			Usage: "log to console's standard error",
		},
		cli.StringFlag{
			Name:  "log-file, l",
			Value: "/var/log/skyring/skyring.log",
			Usage: "Override default log file",
		},
		cli.StringFlag{
			Name:  "log-level",
			Value: DefaultLogLevel.String(),
			Usage: "Set log level",
		},
	}

	app.Before = func(c *cli.Context) error {
		// Set global configuration values
		configDir = c.String("config-dir")
		eventSocket = c.String("event-socket")
		logToStderr = c.Bool("log-to-stderr")
		logFile = c.String("log-file")
		logLevel = c.String("log-level")
		providersDir = c.String("providers-dir")
		return nil
	}

	app.Action = func(c *cli.Context) {
		args := c.Args()
		if len(args) != 0 {
			fmt.Fprintf(os.Stderr, "invalid arguments: [%s]\n", strings.Join(args, " "))
			os.Exit(-1)
		}
		start()
	}

	app.Run(os.Args)
}

func start() {
	var level logging.Level
	var err error
	if level, err = logging.LogLevel(logLevel); err != nil {
		fmt.Fprintf(os.Stderr, "log level error. %s\n", err)
		fmt.Fprintf(os.Stderr, "Using default log level %s\n", DefaultLogLevel)
		level = DefaultLogLevel
	}
	// TODO: use logToStderr when deamonizing.
	if err := logger.Init(logFile, true, level); err != nil {
		panic(fmt.Sprintf("log init failed. %s", err))
	}

	conf.LoadAppConfiguration(path.Join(configDir, ConfigFile))
	conf.SystemConfig.Logging.LogToStderr = logToStderr
	conf.SystemConfig.Logging.Filename = logFile
	conf.SystemConfig.Logging.Level = level

	application := skyring.NewApp(configDir, providersDir)
	if application == nil {
		logger.Get().Error("Unable to start application")
		os.Exit(1)
	}

	// Create router for defining the routes.
	//do not allow any routes unless defined.
	router := mux.NewRouter().StrictSlash(true)

	//Load the autheticated routes
	if err := application.SetRoutes(router); err != nil {
		logger.Get().Error("Unable to create http server endpoints: %s", err)
		os.Exit(1)
	}

	if err := application.InitializeNodeManager(conf.SystemConfig.NodeManagementConfig); err != nil {
		logger.Get().Error("Unable to create node manager")
		os.Exit(1)
	}

	// Use negroni to add middleware.  Here we add the standard
	// middlewares: Recovery, Logger and static file serve which come with
	// Negroni
	n := negroni.Classic()

	logger.Get().Info("Starting event listener")
	go event.StartListener(eventSocket)

	//Check if Port is provided, otherwise use dafault 8080
	//If host is not provided, it binds on all IPs
	if conf.SystemConfig.Config.HttpPort == 0 {
		conf.SystemConfig.Config.HttpPort = 8080
	}

	// Create DB session
	if err := db.InitDBSession(conf.SystemConfig.DBConfig); err != nil {
		logger.Get().Error("Unable to initialize DB")
		os.Exit(1)
	}
	if err := db.InitMonitoringDB(conf.SystemConfig.TimeSeriesDBConfig); err != nil {
		logger.Get().Error("Unable to initialize monitoring DB")
		os.Exit(1)
	}

	//Initialize the auth provider
	if err := application.InitializeAuth(conf.SystemConfig.Authentication, n); err != nil {
		logger.Get().Error("Unable to initialize the authentication provider: %s", err)
		os.Exit(1)
	}

	n.UseHandler(router)

	logger.Get().Info("start listening on %s : %s", conf.SystemConfig.Config.Host, strconv.Itoa(conf.SystemConfig.Config.HttpPort))

	logger.Get().Critical("Error: %s", http.ListenAndServe(conf.SystemConfig.Config.Host+":"+strconv.Itoa(conf.SystemConfig.Config.HttpPort), n))
}
