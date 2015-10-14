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
package skyring

import (
	"encoding/json"
	"github.com/op/go-logging"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/ssh"
	"net"
	"net/http"
)

log := logger.Get()

func (a *App) GET_SshFingerprint(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	fingerprint := make(map[string]string)
	fingerprint["sshfingerprint"], _ = ssh.GetFingerprint(hostname, 22)
	// TODO: handle error and return response properly
	json.NewEncoder(w).Encode(fingerprint)
}

func GET_LookupNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	if host_addrs, err := net.LookupHost(hostname); err == nil {
		if iaddrs, err := net.InterfaceAddrs(); err != nil {
			log.Error("Error getting the local host subnet details")
			json.NewEncoder(w).Encode(host_addrs)
			return
		} else {
			var ret_addrs []string
			for _, host_addr := range host_addrs {
				for _, iaddr := range iaddrs {
					if ipnet, ok := iaddr.(*net.IPNet); ok &&
						!ipnet.IP.IsLoopback() &&
						ipnet.IP.To4() != nil &&
						ipnet.Contains(net.ParseIP(host_addr)) {
						ret_addrs = append(ret_addrs, host_addr)
					}
				}
			}
			if len(ret_addrs) == 0 {
				// In case of geo replication none of the host IPs might fall under skyring
				// server's subnet. So return the output of host lookup only
				json.NewEncoder(w).Encode(host_addrs)
			} else {
				json.NewEncoder(w).Encode(ret_addrs)
			}
			return
		}
	}
}
