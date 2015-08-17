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
package common

import (
	"fmt"
	"github.com/emicklei/go-restful"
	"io"
	"skyring/apps"
	"skyring/conf"
	"skyring/plugins"
)

type CommonApp struct {
	//plugin *glusterplugin.GlusterPlugin
	plugin map[string]plugin.ProviderInterface
}

func init() {
	app.RegisterApp("common", func(config string) (app.Application, error) {
		return NewCommonApp(config), nil
	})
}

func NewCommonApp(config string) *CommonApp {
	capp := &CommonApp{}
	capp.plugin = make(map[string]plugin.ProviderInterface)

	var err error
	pluginCollection := conf.LoadPluginConfiguration(config)
	fmt.Println(pluginCollection)

	for _, element := range pluginCollection.Plugins {
		fmt.Println(element)
		capp.plugin[element.Name], err = plugin.InitPlugin(element.Name, "")
	}

	if err != nil {

	}

	return capp
}

func (a *CommonApp) SetRoutes(container *restful.Container) error {
	ws := new(restful.WebService)
	ws.
		Path("/Hello").
		Doc("USM Appp Hello").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML) // you can specify this per route as well

	ws.Route(ws.GET("").To(a.Hello).
		// docs
		Doc("Common Hello").
		Operation("commonHello")) // on the response

	container.Add(ws)
	return nil

}

func (a *CommonApp) Hello(request *restful.Request, response *restful.Response) {

	fmt.Println("In App Hello")
	rslt, err := a.plugin["gluster"].SayHi("Gluster")
	//a.plugin["ceph"].SayHi("Thomas")
	fmt.Println("rslt", rslt)
	fmt.Println("rslt", err)
	rslt, err = a.plugin["ceph"].SayHi("Ceph")
	fmt.Println("rslt", rslt)
	io.WriteString(response, "Hello Common")
}
