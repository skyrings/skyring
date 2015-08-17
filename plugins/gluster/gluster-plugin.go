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
package glusterplugin

import (
	"fmt"
	"github.com/natefinch/pie"
	"io"
	"net/rpc/jsonrpc"
	"os"
	"skyring/plugins"
)

type GlusterPlugin plugin.Plugin

func init() {
	plugin.RegisterPlugin("gluster", func(config io.Reader) (plugin.ProviderInterface, error) {
		return NewGlusterPlugin(), nil
	})
}

func NewGlusterPlugin() *GlusterPlugin {

	//gluster plugin
	path := "./plugin_provider"

	glusterclient, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, path)
	if err != nil {
		fmt.Println("Error running plugin:")
	}

	gplug := &GlusterPlugin{}
	gplug.Name = "gluster"
	gplug.Client = glusterclient
	return gplug
}

func (p GlusterPlugin) SayHi(name string) (result string, err error) {

	fmt.Println("In Gluster SayHi")
	err = p.Client.Call("Plugin.SayHi", name, &result)
	return result, err
}

func (p GlusterPlugin) SayBye(name string) (result string, err error) {
	err = p.Client.Call("Plugin2.SayBye", name, &result)
	return result, err
}
