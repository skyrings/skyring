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
	"github.com/skyrings/skyring/apps"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/event"
	"github.com/skyrings/skyring/tools/logger"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
)

const (
	// ConfigFile default configuration file
	ConfigFile = "skyring.conf"
	// DefaultLogLevel default log level
	DefaultLogLevel = logging.DEBUG
)

var (
	configDir     string
	logFile       string
	logToStderr   bool
	logLevel      string
	providersDir  string
	eventSocket   string
	staticFileDir string
	websocketPort string
	httpPort      string
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
			Value: "/var/run/.skyring-event",
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
		cli.StringFlag{
			Name:   "static-file-dir, s",
			Value:  "/usr/share/skyring/webapp",
			Usage:  "Override default static file serve directory",
			EnvVar: "SKYRING_STATICFILEDIR",
		},
		cli.StringFlag{
			Name:  "websocket-port",
			Value: "8081",
			Usage: "Websocket server port",
		},
		cli.StringFlag{
			Name:  "http-port",
			Value: "8080",
			Usage: "Http server port",
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
		staticFileDir = c.String("static-file-dir")
		websocketPort = c.String("websocket-port")
		httpPort = c.String("http-port")
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
	var (
		application app.Application
	)
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
	conf.SystemConfig.Config.HttpPort = httpPort

	application = skyring.NewApp(configDir, providersDir)
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

	// Use negroni to add middleware.  Here we add the standard
	// middlewares: Recovery, Logger and static file serve which come with
	// Negroni
	n := negroni.New(
		negroni.NewRecovery(),
		negroni.NewLogger(),
		negroni.NewStatic(http.Dir(staticFileDir)),
	)
	n.UseHandler(router)

	//Initialize the application, db, auth etc
	if err := application.InitializeApplication(conf.SystemConfig); err != nil {
		logger.Get().Error("Unable to initialize the application")
		os.Exit(1)
	}

	logger.Get().Info("Starting event listener")
	go event.StartListener(eventSocket)

	// Starting the WebSocket server
	event.StartBroadcaster(websocketPort)

	logger.Get().Info("start listening on %s : %s", conf.SystemConfig.Config.Host, httpPort)

	go http.ListenAndServe(conf.SystemConfig.Config.Host+":"+httpPort, n)

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)
	go func() {
		sig := <-sigs
		fmt.Println(sig)
		done <- true
	}()
	<-done
	os.Remove(eventSocket)
}
