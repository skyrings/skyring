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
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"strconv"
	"strings"
)

func GetEvents(rw http.ResponseWriter, req *http.Request) {
	var filter bson.M = make(map[string]interface{})

	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	node_name := req.URL.Query().Get("node-name")
	cluster_name := req.URL.Query().Get("cluster-name")
	severity := req.URL.Query().Get("severity")
	acked := req.URL.Query().Get("acked")

	if len(node_name) != 0 {
		filter["nodename"] = node_name
	}
	if len(cluster_name) != 0 {
		filter["cluster_name"] = cluster_name
	}

	var event_severity = map[string]models.AlarmStatus{
		"indeterminate": models.ALARM_STATUS_INDETERMINATE,
		"critical":      models.ALARM_STATUS_CRITICAL,
		"major":         models.ALARM_STATUS_MAJOR,
		"minor":         models.ALARM_STATUS_MINOR,
		"warning":       models.ALARM_STATUS_WARNING,
		"cleared":       models.ALARM_STATUS_CLEARED,
	}

	if len(severity) != 0 {
		if s, ok := event_severity[severity]; !ok {
			logger.Get().Error("%s-Un-supported query param: %v", ctxt, severity)
			HttpResponse(rw, http.StatusInternalServerError, fmt.Sprintf("Un-supported query param: %s", severity))
			return
		} else {
			filter["severity"] = s
		}
	}
	if len(acked) != 0 {
		if strings.ToLower(acked) == "true" {
			filter["acked"] = true
		} else if strings.ToLower(acked) == "false" {
			filter["acked"] = false
		} else {
			logger.Get().Error("%s-Un-supported query param: %v", ctxt, acked)
			HttpResponse(rw, http.StatusInternalServerError, fmt.Sprintf("Un-supported query param: %s", acked))
			return
		}
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_APP_EVENTS)
	var events []models.AppEvent
	pageNo, pageNoErr := strconv.Atoi(req.URL.Query().Get("page-no"))
	pageSize, pageSizeErr := strconv.Atoi(req.URL.Query().Get("page-size"))
	if err := coll.Find(filter).Sort("-timestamp").All(&events); err != nil {
		logger.Get().Error("%s-Error getting record from DB: %v", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	if len(events) == 0 {
		json.NewEncoder(rw).Encode([]models.AppEvent{})
	} else {
		var startIndex int = 0
		var endIndex int = models.EVENTS_PER_PAGE
		if pageNoErr == nil && pageSizeErr == nil {
			startIndex, endIndex = Paginate(pageNo, pageSize, models.EVENTS_PER_PAGE)
		}
		if endIndex > len(events) {
			endIndex = len(events) - 1
		}
		json.NewEncoder(rw).Encode(
			struct {
				Totalcount int               `json:"totalcount"`
				Startindex int               `json:"startindex"`
				Endindex   int               `json:"endindex"`
				Events     []models.AppEvent `json:"events"`
			}{len(events), startIndex, endIndex, events[startIndex : endIndex+1]})
	}
}

func GetEventById(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(r)
	event_id_str := vars["event-id"]
	event_id, err := uuid.Parse(event_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing event id: %s", ctxt, event_id_str)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing event id: %s", event_id_str))
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_APP_EVENTS)
	var event models.AppEvent
	if err := collection.Find(bson.M{"eventid": *event_id}).One(&event); err != nil {
		logger.Get().Error("%s-Error getting the event detail for %v. error: %v", ctxt, *event_id, err)
	}

	if event.Name == "" {
		HttpResponse(w, http.StatusBadRequest, "Event not found")
		logger.Get().Error("%s-Event: %v not found. error: %v", ctxt, *event_id, err)
		return
	} else {
		json.NewEncoder(w).Encode(event)
	}
}
