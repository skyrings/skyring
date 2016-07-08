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
	"io"
	"io/ioutil"
	"net/http"
)

func (a *App) POST_StorageProfiles(w http.ResponseWriter, r *http.Request) {

	var request models.StorageProfile

	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	// Unmarshal the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, models.REQUEST_SIZE_LIMIT))
	if err != nil {
		logger.Get().Error("%s-Error parsing the request: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_CREATED"],
			fmt.Sprintf("Failed to create storage profile"),
			fmt.Sprintf("Failed to create storage profile. Error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to parse the request: %v", err), ctxt)
		return
	}
	if err := json.Unmarshal(body, &request); err != nil {
		logger.Get().Error("%s-Error Unmarshalling the request: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_CREATED"],
			fmt.Sprintf("Failed to create storage profile"),
			fmt.Sprintf("Failed to create storage profile. Error: %v", err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Unable to unmarshal request: %v", err), ctxt)
		return
	}
	if (request == models.StorageProfile{}) {
		logger.Get().Error("%s-Invalid request", ctxt)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_CREATED"],
			fmt.Sprintf("Failed to create storage profile"),
			fmt.Sprintf(
				"Failed to create storage profile. Error: %v",
				fmt.Errorf("Invalid request")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusBadRequest, "Invalid request", ctxt)
		return
	}
	// Check if storage profile already added
	if _, err := GetDbProvider().StorageProfileInterface().StorageProfile(ctxt, request.Name); err == nil {
		logger.Get().Error("%s-Storage profile already added: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_CREATED"],
			fmt.Sprintf("Failed to create storage profile: %s", request.Name),
			fmt.Sprintf("Failed to create storage profile: %s. Error: %v", request.Name, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusMethodNotAllowed, "Storage profile already added", ctxt)
		return

	}

	if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile(ctxt, request); err != nil {
		logger.Get().Error("%s-Storage profile add failed: %v", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_CREATED"],
			fmt.Sprintf("Failed to create storage profile: %s", request.Name),
			fmt.Sprintf("Failed to create storage profile:%s. Error: %v", request.Name, err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log create storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusInternalServerError, err.Error(), ctxt)
		return
	}
	if err := logAuditEvent(EventTypes["STORAGE_PROFILE_CREATED"],
		fmt.Sprintf("Created storage profile: %s", request.Name),
		fmt.Sprintf("Created storage profile: %s", request.Name),
		nil,
		nil,
		models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
		nil,
		false,
		ctxt); err != nil {
		logger.Get().Error("%s- Unable to log create storage profile event. Error: %v", ctxt, err)
	}
	return
}

func (a *App) GET_StorageProfiles(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("%s-Error Getting the context. error: %v", ctxt, err)
	}
	sProfiles, err := GetDbProvider().StorageProfileInterface().StorageProfiles(ctxt, nil, models.QueryOps{})
	if err != nil {
		HttpResponse(w, http.StatusInternalServerError, err.Error(), ctxt)
		return
	}
	if len(sProfiles) == 0 {
		json.NewEncoder(w).Encode(models.StorageProfile{})
	} else {
		json.NewEncoder(w).Encode(sProfiles)
	}
}

func (a *App) GET_StorageProfile(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("%s-Error Getting the context. error: %v", ctxt, err)
	}
	vars := mux.Vars(r)
	name := vars["name"]
	sprofile, err := GetDbProvider().StorageProfileInterface().StorageProfile(ctxt, name)
	if err != nil {
		HttpResponse(w, http.StatusNotFound, err.Error(), ctxt)
		return
	}
	json.NewEncoder(w).Encode(sprofile)
	return
}

func (a *App) DELETE_StorageProfile(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("%s-Error Getting the context. error: %v", ctxt, err)
	}
	vars := mux.Vars(r)
	sprofile, err := GetDbProvider().StorageProfileInterface().StorageProfile(ctxt, vars["name"])
	if err != nil {
		logger.Get().Error("%s-Unable to Get Storage Profile:%s", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_REMOVED"],
			fmt.Sprintf("Failed to delete storage profile: %s", vars["name"]),
			fmt.Sprintf("Failed to delete storage profile: %s. Error: %v", vars["name"], err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusInternalServerError, err.Error(), ctxt)
		return
	}
	if sprofile.Default {
		logger.Get().Error("%s-Default Storage Profile cannot be deleted:%s", err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_REMOVED"],
			fmt.Sprintf("Failed to delete storage profile: %s", vars["name"]),
			fmt.Sprintf(
				"Failed to delete storage profile: %s. Error: %v",
				vars["name"],
				fmt.Errorf("Default storage profile cannot be deleted")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusInternalServerError, "Default Storage Profile Cannot be Deleted", ctxt)
		return
	}
	if err := GetDbProvider().StorageProfileInterface().DeleteStorageProfile(ctxt, vars["name"]); err != nil {
		logger.Get().Error("%s-Unable to delete Storage Profile:%s", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_REMOVED"],
			fmt.Sprintf("Failed to delete storage profile: %s", vars["name"]),
			fmt.Sprintf("Failed to delete storage profile: %s. Error: %v", vars["name"], err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log delete storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := logAuditEvent(EventTypes["STORAGE_PROFILE_REMOVED"],
		fmt.Sprintf("Deleted storage profile: %s", vars["name"]),
		fmt.Sprintf("Deleted storage profile: %s.", vars["name"]),
		nil,
		nil,
		models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
		nil,
		false,
		ctxt); err != nil {
		logger.Get().Error("%s- Unable to log delete storage profile event. Error: %v", ctxt, err)
	}
}

func (a *App) PATCH_StorageProfile(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("%s-Error Getting the context. error: %v", ctxt, err)
	}
	vars := mux.Vars(r)
	sprofile, err := GetDbProvider().StorageProfileInterface().StorageProfile(ctxt, vars["name"])
	if err != nil {
		logger.Get().Error("%s-Unable to Get Storage Profile:%s", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_UPDATED"],
			fmt.Sprintf("Failed to update storage profile: %s", vars["name"]),
			fmt.Sprintf("Failed to update storage profile: %s. Error: %v", vars["name"], err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusInternalServerError, err.Error(), ctxt)
		return
	}
	if sprofile.Default {
		logger.Get().Error("%s-Default Storage Profile cannot be modified:%s", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_UPDATED"],
			fmt.Sprintf("Failed to update storage profile: %s", vars["name"]),
			fmt.Sprintf(
				"Failed to update storage profile: %s. Error: %v",
				vars["name"],
				fmt.Errorf("Default storage profile cannot be updated")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusInternalServerError, "Default Storage Profile Cannot be Modified", ctxt)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_UPDATED"],
			fmt.Sprintf("Failed to update storage profile: %s", vars["name"]),
			fmt.Sprintf("Failed to update storage profile: %s. Error: %v", vars["name"], err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusInternalServerError, err.Error(), ctxt)
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data:%s", ctxt, err)
		if err := logAuditEvent(EventTypes["STORAGE_PROFILE_UPDATED"],
			fmt.Sprintf("Failed to update storage profile: %s", vars["name"]),
			fmt.Sprintf("Failed to update storage profile: %s. Error: %v", vars["name"], err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
			nil,
			false,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update storage profile event. Error: %v", ctxt, err)
		}
		HttpResponse(w, http.StatusInternalServerError, err.Error(), ctxt)
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
		if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile(ctxt, sprofile); err != nil {
			logger.Get().Error("%s-Storage profile update failed: %v", ctxt, err)
			if err := logAuditEvent(EventTypes["STORAGE_PROFILE_UPDATED"],
				fmt.Sprintf("Failed to update storage profile: %s", vars["name"]),
				fmt.Sprintf("Failed to update storage profile: %s. Error: %v", vars["name"], err),
				nil,
				nil,
				models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
				nil,
				false,
				ctxt); err != nil {
				logger.Get().Error("%s- Unable to log update storage profile event. Error: %v", ctxt, err)
			}
			HttpResponse(w, http.StatusInternalServerError, err.Error(), ctxt)
			return
		}
	}
	if err := logAuditEvent(EventTypes["STORAGE_PROFILE_UPDATED"],
		fmt.Sprintf("Updated storage profile: %s", vars["name"]),
		fmt.Sprintf("Update storage profile: %s.", vars["name"]),
		nil,
		nil,
		models.NOTIFICATION_ENTITY_STORAGE_PROFILE,
		nil,
		false,
		ctxt); err != nil {
		logger.Get().Error("%s- Unable to log update storage profile event. Error: %v", ctxt, err)
	}
}

func AddDefaultProfiles() error {
	//sas
	if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile("", models.StorageProfile{Name: models.DefaultProfile1, Priority: models.DefaultPriority, Default: true}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	//ssd
	if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile("", models.StorageProfile{Name: models.DefaultProfile2, Priority: models.DefaultPriority, Default: true}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	//general
	if err := GetDbProvider().StorageProfileInterface().SaveStorageProfile("", models.StorageProfile{Name: models.DefaultProfile3, Priority: models.DefaultPriority, Default: true}); err != nil {
		logger.Get().Error("Default Storage profile add failed: %v", err)
	}
	return nil
}
