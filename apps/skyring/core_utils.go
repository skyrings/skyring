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
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/tools/ssh"
	"net/http"
)

func GET_SshFingerprint(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hostname := vars["hostname"]

	fingerprint := make(map[string]string)
	fingerprint["sshfingerprint"], _ := ssh.GetFingerprint(hostname, 22)
	// TODO: handle error and return response properly
	json.NewEncoder(w).Encode(fingerprint)
}
