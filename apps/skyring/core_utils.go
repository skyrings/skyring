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
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/utils"
	"net"
	"net/http"
)

func GET_SshFingerprint(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	fingerprint := make(map[string]string)
	fingerprint["sshfingerprint"] = GetCoreNodeManager().GetNodeSshFingerprint(hostname)
	json.NewEncoder(w).Encode(fingerprint)
}

func GET_LookupNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	host_addrs, err := net.LookupHost(hostname)
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, "Error looking up the node detail")
		return
	}

	iaddrs, err := net.InterfaceAddrs()
	if err != nil {
		glog.Errorf("Error getting the local host subnet details")
	}

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
}
