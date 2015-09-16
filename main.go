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
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/apps"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/utils"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

const (
	// ConfigFile default configuration file
	ConfigFile = "skyring.conf"
)

var (
	configDir    string
	logDir       string
	logToStderr  bool
	logLevel     int
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
			Name:   "log-dir, l",
			Value:  "/var/log/skyring",
			Usage:  "Override default log directory",
			EnvVar: "SKYRING_LOGDIR",
		},
		cli.IntFlag{
			Name:  "log-level",
			Value: 0,
			Usage: "Enable V-leveled logging at the specified level",
		},
	}

	app.Before = func(c *cli.Context) error {
		// Set global configuration values
		configDir = c.String("config-dir")
		eventSocket = c.String("event-socket")
		logToStderr = c.Bool("log-to-stderr")
		logDir = c.String("log-dir")
		logLevel = c.Int("log-level")
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
	defer glog.Flush()

	var (
		application app.Application
		err         error
	)

	appCollection := conf.LoadAppConfiguration(path.Join(configDir, ConfigFile))
	appCollection.Logging.Logtostderr = logToStderr
	appCollection.Logging.Log_dir = logDir
	appCollection.Logging.V = logLevel

	util.InitLogs(appCollection.Logging)

	application = skyring.NewApp(configDir, providersDir)

	if application == nil {
		glog.Errorf("Unable to start application")
		os.Exit(1)
	}
	// Create a router and do not allow any routes
	// unless defined.
	router := mux.NewRouter().StrictSlash(true)
	err = application.SetRoutes(router)
	if err != nil {
		glog.Errorf("Unable to create http server endpoints")
		os.Exit(1)
	}

	err = application.InitializeNodeManager(appCollection.NodeManagementConfig)
	if err != nil {
		glog.Errorf("Unable to create node manager")
		os.Exit(1)
	}

	glog.Info("Starting event listener")
	go util.StartEventListener(eventSocket)

	//Check if Port is provided, otherwise use dafault 8080
	//If host is not provided, it binds on all IPs
	if appCollection.Config.HttpPort == 0 {
		appCollection.Config.HttpPort = 8080
	}
	glog.Infof("start listening on %s : %s", appCollection.Config.Host, strconv.Itoa(appCollection.Config.HttpPort))

	glog.Fatalf("Error: %s", http.ListenAndServe(appCollection.Config.Host+":"+strconv.Itoa(appCollection.Config.HttpPort), router))
}
