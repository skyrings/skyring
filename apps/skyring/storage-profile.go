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
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/utils"
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
	if _, err := GetDbProvider().StorageProfileInterface().StorageProfile(request.Name); err == nil {
		logger.Get().Error("Storage profile already added: %v", err)
		util.HttpResponse(w, http.StatusMethodNotAllowed, "Storage profile already added")
		return

	}

	if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile(request); err != nil {
		logger.Get().Error("Storage profile add failed: %v", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	return
}

func (a *App) GET_StorageProfiles(w http.ResponseWriter, r *http.Request) {
	sProfiles, err := GetDbProvider().StorageProfileInterface().StorageProfiles(nil, models.QueryOps{})
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
	sprofile, err := GetDbProvider().StorageProfileInterface().StorageProfile(name)
	if err != nil {
		util.HttpResponse(w, http.StatusNotFound, err.Error())
		return
	}
	json.NewEncoder(w).Encode(sprofile)
	return
}

func (a *App) DELETE_StorageProfile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sprofile, err := GetDbProvider().StorageProfileInterface().StorageProfile(vars["name"])
	if err != nil {
		logger.Get().Error("Unable to Get Storage Profile:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sprofile.Default {
		logger.Get().Error("Default Storage Profile cannot be deleted:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Default Storage Profile Cannot be Deleted")
		return
	}
	if err := GetDbProvider().StorageProfileInterface().DeleteStorageProfile(vars["name"]); err != nil {
		logger.Get().Error("Unable to delete Storage Profile:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

}

func (a *App) PATCH_StorageProfile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sprofile, err := GetDbProvider().StorageProfileInterface().StorageProfile(vars["name"])
	if err != nil {
		logger.Get().Error("Unable to Get Storage Profile:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sprofile.Default {
		logger.Get().Error("Default Storage Profile cannot be modified:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, "Default Storage Profile Cannot be Modified")
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		util.HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	var updated bool
	if val, ok := m["priority"]; ok {
		sprofile.Priority = int(val.(float64))
		updated = true
	}
	if val, ok := m["rule"]; ok {
		if ruleM, ok := val.(map[string]interface{}); ok {
			if ruleV, ok := ruleM["disktype"]; ok {
				sprofile.Rule.Type = models.DiskType(ruleV.(float64))
				updated = true
			}
			if ruleV, ok := ruleM["speed"]; ok {
				sprofile.Rule.Speed = int(ruleV.(float64))
				updated = true
			}
		}
	}
	if updated {
		if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile(sprofile); err != nil {
			logger.Get().Error("Storage profile update failed: %v", err)
			util.HttpResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
}

func AddDefaultProfiles() error {
	//sas
	if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile(models.StorageProfile{Name: models.DefaultProfile1, Priority: models.DefaultPriority, Default: true}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	//ssd
	if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile(models.StorageProfile{Name: models.DefaultProfile2, Priority: models.DefaultPriority, Default: true}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	//general
	if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile(models.StorageProfile{Name: models.DefaultProfile3, Priority: models.DefaultPriority, Default: true}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	return nil
}
