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
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring/apps"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/event"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const (
	// ConfigFile default configuration file
	ConfigFile = "skyring.conf"
	// DefaultLogLevel default log level
	DefaultLogLevel = logging.DEBUG
	//SkyringEventSocketFile skyring event socket file
	SkyringEventSocketFile = "/var/run/.skyring-event"
)

var (
	configDir     string
	logFile       string
	logToStderr   bool
	logLevel      string
	providersDir  string
	staticFileDir string
	websocketPort string
	httpPort      int
	sslPort       int
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
		cli.IntFlag{
			Name:  "http-port",
			Value: 8080,
			Usage: "Http server port",
		},
		cli.IntFlag{
			Name:  "ssl-port",
			Value: 10443,
			Usage: "SSL server port",
		},
	}

	app.Before = func(c *cli.Context) error {
		// Set global configuration values
		configDir = c.String("config-dir")
		logToStderr = c.Bool("log-to-stderr")
		logFile = c.String("log-file")
		logLevel = c.String("log-level")
		providersDir = c.String("providers-dir")
		staticFileDir = c.String("static-file-dir")
		websocketPort = c.String("websocket-port")
		httpPort = c.Int("http-port")
		sslPort = c.Int("ssl-port")
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

func runHTTP(addr string, ssl bool, sslAddr string, sslParams map[string]string, n *negroni.Negroni) {
	// Start ssl server
	if ssl {
		go func() {
			logger.Get().Info("Started listening on %s", sslAddr)
			if err := http.ListenAndServeTLS(sslAddr, sslParams["cert"], sslParams["key"], n); err != nil {
				logger.Get().Fatalf("Unable to start the webserver. err: %v", err)
			}
		}()
	} else {
		// Start http server
		go func() {
			logger.Get().Info("Started listening on %s", addr)
			if err := http.ListenAndServe(addr, n); err != nil {
				logger.Get().Fatalf("Unable to start the webserver. err: %v", err)
			}
		}()
	}
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
	if err := logger.Init("skyring", logFile, true, level); err != nil {
		panic(fmt.Sprintf("log init failed. %s", err))
	}

	conf.LoadAppConfiguration(configDir, ConfigFile)
	conf.SystemConfig.Logging.LogToStderr = logToStderr
	conf.SystemConfig.Logging.Filename = logFile
	conf.SystemConfig.Logging.Level = level
	conf.SystemConfig.Config.HttpPort = httpPort
	conf.SystemConfig.Config.SslPort = sslPort

	application = skyring.NewApp(configDir, providersDir)
	if application == nil {
		logger.Get().Fatalf("Unable to start application")
	}

	// Create router for defining the routes.
	//do not allow any routes unless defined.
	router := mux.NewRouter().StrictSlash(true)

	//Load the autheticated routes
	if err := application.SetRoutes(router); err != nil {
		logger.Get().Fatalf("Unable to create http server endpoints. error: %v", err)
	}

	// Use negroni to add middleware.  Here we add the standard
	// middlewares: Recovery, Logger and static file serve which come with
	// Negroni
	n := negroni.New(
		negroni.NewRecovery(),
		negroni.NewLogger(),
		negroni.NewStatic(http.Dir(staticFileDir)),
	)
	//Add the logging context middleware
	n.Use(negroni.HandlerFunc(application.LoggingContext))

	n.UseHandler(router)

	//Initialize the application, db, auth etc
	if err := application.InitializeApplication(conf.SystemConfig, configDir); err != nil {
		logger.Get().Fatalf("Unable to initialize the application. err: %v", err)
	}

	logger.Get().Info("Starting the providers")
	//Load providers and routes
	//Load all the files present in the config path
	if err := application.StartProviders(configDir, providersDir); err != nil {
		logger.Get().Fatalf("Unable to initialize the Providers. err: %v", err)
	}

	logger.Get().Info("Starting event listener")
	go event.StartListener(SkyringEventSocketFile)

	// Starting the WebSocket server
	event.StartBroadcaster(
		websocketPort,
		conf.SystemConfig.Config.SSLEnabled,
		map[string]string{
			"cert": conf.SystemConfig.Config.SslCert,
			"key":  conf.SystemConfig.Config.SslKey,
		})

	runHTTP(
		fmt.Sprintf("%s:%d", conf.SystemConfig.Config.Host, httpPort),
		conf.SystemConfig.Config.SSLEnabled,
		fmt.Sprintf("%s:%d", conf.SystemConfig.Config.Host, sslPort),
		map[string]string{
			"cert": conf.SystemConfig.Config.SslCert,
			"key":  conf.SystemConfig.Config.SslKey,
		},
		n)

	if err := application.PostInitApplication(conf.SystemConfig); err != nil {
		logger.Get().Fatalf("Unable to run Post init. err: %v", err)
	}

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)
	go func() {
		sig := <-sigs
		logger.Get().Info("Signal:", sig)
		done <- true
	}()
	<-done
	os.Remove(SkyringEventSocketFile)
}
