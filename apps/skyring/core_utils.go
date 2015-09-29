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
	"github.com/skyrings/skyring/utils"
	"net"
	"net/http"
)

func GET_SshFingerprint(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	hostname := params.Get("hostname")

	if hostname == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Host name not provided")
		return
	}

	fingerprint := make(map[string]string)
	fingerprint["sshfingerprint"] = GetCoreNodeManager().GetNodeSshFingerprint(hostname)
	json.NewEncoder(w).Encode(fingerprint)
}

func GET_LookupNode(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	hostname := params.Get("hostname")

	if hostname == "" {
		util.HttpResponse(w, http.StatusBadRequest, "Host name not provided")
		return
	}

	addrs, err := net.LookupHost(hostname)
	if err != nil {
		util.HttpResponse(w, http.StatusBadRequest, "Error looking up the node detail")
		return
	}

	json.NewEncoder(w).Encode(addrs)
}
