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
	"fmt"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"net/http"
)

func (a *App) About(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	var sys_capabilities conf.System_Capabilities
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_SYSTEM_CAPABILITIES)
	if err := coll.Find(nil).One(&sys_capabilities); err != nil {
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error in retrieving System Capabilities detail. error: %v", err))
		logger.Get().Error("%s-Error in retrieving System Capabilities detail. error: %v", ctxt, err)
		return
	}
	if err := json.NewEncoder(w).Encode(sys_capabilities); err != nil {
		logger.Get().Error("%s-Error encoding data: %v", ctxt, err)
		HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
}
