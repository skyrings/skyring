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
package ceph

import (
	"fmt"
	"github.com/emicklei/go-restful"
	"io"
	"skyring/apps"
	"skyring/plugins"
)

type CephApp struct {
	plugin plugin.ProviderInterface
	//plugin map[string]plugin.ProviderInterface
}

func init() {
	app.RegisterApp("ceph", func(config string) (app.Application, error) {
		return NewCephApp(config), nil
	})
}

func NewCephApp(config string) *CephApp {

	capp := &CephApp{}
	var err error
	capp.plugin, err = plugin.InitPlugin("ceph", "")

	if err != nil {

	}

	return capp
}

func (a *CephApp) SetRoutes(container *restful.Container) error {
	ws := new(restful.WebService)
	ws.
		Path("/Hello/Ceph").
		Doc("USM Ceph Appp Hello").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML) // you can specify this per route as well

	ws.Route(ws.GET("").To(a.HelloCeph).
		// docs
		Doc(" Ceph Hello").
		Operation("cephHello")) // on the response

	container.Add(ws)
	return nil

}

func (a *CephApp) HelloCeph(request *restful.Request, response *restful.Response) {

	rslt, err := a.plugin.SayHi("HelloCeph")

	fmt.Println("rslt", rslt)
	fmt.Println("rslt", err)

	io.WriteString(response, "Hello Ceph")
}
