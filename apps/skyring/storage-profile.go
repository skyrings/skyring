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
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/utils"
	"io"
	"io/ioutil"
	"net/http"
)

func (a *App) POST_StorageProfiles(w http.ResponseWriter, r *http.Request) {

	var request models.StorageProfile
	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("Error parsing the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err))
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("Error Unmarshalling the request: %v", err)
		util.HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err))
		return
	}
	if (request == models.StorageProfile{}) {
		logger.Get().Error("Invalid request")
		util.HttpResponse(w, http.StatusBadRequest, "Invalid request")
		return
	}
	// Check if storage profile already added
	if _, err := GetDbProvider().StorageProfile(request.Name); err == nil {
		logger.Get().Error("Storage profile already added: %v", err)
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Storage profile already added")
		return

	}

	if err := GetDbProvider().SaveStorageProfile(request); err != nil {
		logger.Get().Error("Storage profile add failed: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	return
}

func (a *App) GET_StorageProfiles(w http.ResponseWriter, r *http.Request) {
	sProfiles, err := GetDbProvider().StorageProfiles()
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(sProfiles) == 0 {
		json.NewEncoder(w).Encode(models.StorageProfile{})
	} else {
		json.NewEncoder(w).Encode(sProfiles)
	}
}

func (a *App) GET_StorageProfile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	sprofile, err := GetDbProvider().StorageProfile(name)
	if err != nil {
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(sprofile)
	return
}

func AddDefaultProfiles() error {
	//sas
	if err := GetDbProvider().SaveStorageProfile(models.StorageProfile{Name: models.DefaultProfile1, Priority: models.DefaultPriority}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	//ssd
	if err := GetDbProvider().SaveStorageProfile(models.StorageProfile{Name: models.DefaultProfile2, Priority: models.DefaultPriority}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	//general
	if err := GetDbProvider().SaveStorageProfile(models.StorageProfile{Name: models.DefaultProfile3, Priority: models.DefaultPriority}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	return nil
}
