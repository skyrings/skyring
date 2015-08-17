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
package gluster

import (
	"fmt"
	"github.com/emicklei/go-restful"
	"io"
	"skyring/apps"
	"skyring/plugins"
)

type GlusterApp struct {
	plugin plugin.ProviderInterface
	//plugin map[string]plugin.ProviderInterface
}

func init() {
	app.RegisterApp("gluster", func(config string) (app.Application, error) {
		return NewGlusterApp(config), nil
	})
}

func NewGlusterApp(config string) *GlusterApp {
	gapp := &GlusterApp{}
	var err error
	gapp.plugin, err = plugin.InitPlugin("gluster", "")
	if err != nil {

	}
	return gapp
}

func (a *GlusterApp) SetRoutes(container *restful.Container) error {
	ws := new(restful.WebService)
	ws.
		Path("/Hello/gluster").
		Doc("USM Gluster Appp Hello").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML) // you can specify this per route as well

	ws.Route(ws.GET("").To(a.HelloGluster).
		// docs
		Doc("Say Hello").
		Operation("glusterhello")) // on the response

	container.Add(ws)
	return nil

}

func (a *GlusterApp) HelloGluster(request *restful.Request, response *restful.Response) {

	rslt, err := a.plugin.SayHi("HelloGluster")

	fmt.Println("rslt", rslt)
	fmt.Println("rslt", err)

	io.WriteString(response, "Hello Gluster")
}
