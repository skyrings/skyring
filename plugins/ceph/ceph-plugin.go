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
package cephplugin

import (
	"fmt"
	"github.com/natefinch/pie"
	"io"
	"net/rpc/jsonrpc"
	"os"
	"skyring/plugins"
)

type CephPlugin plugin.Plugin

func init() {
	plugin.RegisterPlugin("ceph", func(config io.Reader) (plugin.ProviderInterface, error) {
		return NewCephPlugin(), nil
	})
}

func NewCephPlugin() *CephPlugin {

	//ceph plugin
	path := "./plugin_provider"
	cephclient, err := pie.StartProviderCodec(jsonrpc.NewClientCodec, os.Stderr, path)
	if err != nil {
		fmt.Println("Error running plugin:")
	}

	cplug := &CephPlugin{}
	cplug.Name = "ceph"
	cplug.Client = cephclient
	return cplug
}

func (p CephPlugin) SayHi(name string) (result string, err error) {

	fmt.Println("In Ceph SayHi")
	err = p.Client.Call("Plugin.SayHi", name, &result)
	return result, err
}

func (p CephPlugin) SayBye(name string) (result string, err error) {
	err = p.Client.Call("Plugin2.SayBye", name, &result)
	return result, err
}
