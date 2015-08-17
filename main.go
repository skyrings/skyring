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
	"github.com/emicklei/go-restful"
	"github.com/emicklei/go-restful/swagger"
	"log"
	"net/http"
	"os"
	"skyring/apps"
	"skyring/conf"
)

var configfile string

func init() {
	flag.StringVar(&configfile, "config", "", "Configuration file")
}

func NewHandlerContainer() *restful.Container {
	container := restful.NewContainer()
	return container
}

func main() {
	flag.Parse()
	// to see what happens in the package, uncomment the following
	//restful.TraceLogger(log.New(os.Stdout, "[restful] ", log.LstdFlags|log.Lshortfile))

	wsContainer := NewHandlerContainer()

	var application app.Application
	var err error

	// Check configuration file was given
	if configfile == "" {
		fmt.Fprintln(os.Stderr, "Please provide configuration file")
		os.Exit(1)
	}

	// Setup a new applications for common APIs

	appCollection := conf.LoadAppConfiguration(configfile)
	fmt.Println(appCollection)

	for _, element := range appCollection.Apps {
		fmt.Println("Initializing..", element)
		application, err = app.InitApp(element.Name, element.ConfigFilePath)

		err = application.SetRoutes(wsContainer)
		if err != nil {
			fmt.Println("Unable to create http server endpoints")
			os.Exit(1)
		}
	}

	// Install the Swagger Service which provides a nice Web UI on your REST API
	// You need to download the Swagger HTML5 assets and change the FilePath location in the config below.
	// Open http://localhost:8080/apidocs and enter http://localhost:8080/apidocs.json in the api input field.
	//fmt.Println("appCollection.Swagger.Status", appCollection.Swagger.Status)
	if appCollection.Swagger.Status {

		config := swagger.Config{}
		config.WebServices = wsContainer.RegisteredWebServices()
		config.WebServicesUrl = appCollection.Swagger.WebServicesUrl
		config.ApiPath = appCollection.Swagger.ApiPath
		config.SwaggerPath = appCollection.Swagger.SwaggerPath
		config.SwaggerFilePath = appCollection.Swagger.SwaggerFilePath

		swagger.RegisterSwaggerService(config, wsContainer)
	}
	log.Printf("start listening on localhost:8080")
	server := &http.Server{Addr: ":8080", Handler: wsContainer}
	log.Fatal(server.ListenAndServe())
}
