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
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"
	"skyring/apps"
	"skyring/apps/skyring"
	"skyring/conf"
)

var configfile string

func init() {
	flag.StringVar(&configfile, "config", "", "Configuration file")
}

func main() {
	flag.Parse()

	var application app.Application
	var err error

	// Check configuration file was given
	if configfile == "" {
		fmt.Fprintln(os.Stderr, "Please provide configuration file")
		os.Exit(1)
	}

	appCollection := conf.LoadAppConfiguration(configfile)
	application = skyring.NewApp(appCollection.Config.ConfigFilePath)

	if application == nil {
		fmt.Println("Unable to start application")
		os.Exit(1)
	}
	// Create a router and do not allow any routes
	// unless defined.
	router := mux.NewRouter().StrictSlash(true)
	err = application.SetRoutes(router)
	if err != nil {
		fmt.Println("Unable to create http server endpoints")
		os.Exit(1)
	}

	log.Printf("start listening on localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", router))
}
