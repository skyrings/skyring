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
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func GetEvents(rw http.ResponseWriter, req *http.Request) {
	var filter bson.M = make(map[string]interface{})

	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	node_name := req.URL.Query().Get("nodename")
	cluster_name := req.URL.Query().Get("clustername")
	severity := req.URL.Query()["severity"]
	acked := req.URL.Query().Get("acked")
	search_message := req.URL.Query().Get("searchmessage")

	if len(node_name) != 0 {
		filter["nodename"] = node_name
	}
	if len(cluster_name) != 0 {
		filter["clustername"] = cluster_name
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
		var arr []interface{}
		for _, sev := range severity {
			if sev == "" {
				continue
			}
			if s, ok := event_severity[sev]; !ok {
				logger.Get().Error("%s-Un-supported query param: %v", ctxt, sev)
				HttpResponse(rw, http.StatusBadRequest, fmt.Sprintf("Un-supported query param: %s", severity))
				return
			} else {
				arr = append(arr, bson.M{"severity": s})
			}
		}
		if len(arr) != 0 {
			filter["$or"] = arr
		}
	}

	if len(acked) != 0 {
		if strings.ToLower(acked) == "true" {
			filter["acked"] = true
		} else if strings.ToLower(acked) == "false" {
			filter["acked"] = false
		} else {
			logger.Get().Error("%s-Un-supported query param: %v", ctxt, acked)
			HttpResponse(rw, http.StatusBadRequest, fmt.Sprintf("Un-supported query param: %s", acked))
			return
		}
	}
	if len(search_message) != 0 {
		filter["message"] = bson.M{"$regex": search_message, "$options": "$i"}
	}
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_APP_EVENTS)
	var events []models.AppEvent
	pageNo, pageNoErr := strconv.Atoi(req.URL.Query().Get("pageno"))
	pageSize, pageSizeErr := strconv.Atoi(req.URL.Query().Get("pagesize"))

	// time stamp format of RFC3339 = "2006-01-02T15:04:05Z07:00"
	fromDateTime, fromDateTimeErr := time.Parse(time.RFC3339, req.URL.Query().Get("fromdatetime"))
	toDateTime, toDateTimeErr := time.Parse(time.RFC3339, req.URL.Query().Get("todatetime"))
	if len(req.URL.Query().Get("fromdatetime")) != 0 && fromDateTimeErr != nil {
		logger.Get().Error("%s-Un-supported query param: %v.Error: %v", ctxt, req.URL.Query().Get("fromdatetime"), fromDateTimeErr)
		HttpResponse(rw, http.StatusBadRequest, fmt.Sprintf("Un-supported query param for date-time filter: %v", req.URL.Query().Get("fromdatetime")))
		return
	}
	if len(req.URL.Query().Get("todatetime")) != 0 && toDateTimeErr != nil {
		logger.Get().Error("%s-Un-supported query param: %v. Error: %v", ctxt, req.URL.Query().Get("todatetime"), toDateTimeErr)
		HttpResponse(rw, http.StatusBadRequest, fmt.Sprintf("Un-supported query param for date-time filter: %v", req.URL.Query().Get("todatetime")))
		return
	}
	if fromDateTimeErr == nil && toDateTimeErr == nil {
		if toDateTime.Before(fromDateTime) {
			logger.Get().Error("%s-Un-supported query param. From date should be before To date", ctxt)
			HttpResponse(rw, http.StatusBadRequest, fmt.Sprintf("Un-supported query param for date-time filter. From date should be before To date"))
			return
		}

		filter["timestamp"] = bson.M{
			"$gt": fromDateTime.UTC(),
			"$lt": toDateTime.UTC(),
		}
	} else if fromDateTimeErr != nil && toDateTimeErr == nil {
		filter["timestamp"] = bson.M{
			"$lt": toDateTime.UTC(),
		}
	} else if fromDateTimeErr == nil && toDateTimeErr != nil {
		filter["timestamp"] = bson.M{
			"$gt": fromDateTime.UTC(),
		}
	}

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
		// its valid to not mention pageno and page size
		// but pageNoErr and pageSizeErr will have err values
		// if they are not passed. So assigining nil here
		if len(req.URL.Query().Get("pageno")) == 0 {
			pageNoErr = nil
		} else if pageNo <= 0 {
			logger.Get().Error("%s-Un-supported query param: %v", ctxt, pageNo)
			HttpResponse(rw,
				http.StatusBadRequest,
				fmt.Sprintf("Un-supported query param for pagination."+
					" Page number must be greater than 0. Invalid page"+
					" number value: %v", req.URL.Query().Get("pageno")))
			return
		}
		if len(req.URL.Query().Get("pagesize")) == 0 {
			pageSizeErr = nil
		} else if pageSize <= 0 {
			logger.Get().Error("%s-Un-supported query param: %v", ctxt, pageSize)
			HttpResponse(
				rw,
				http.StatusBadRequest,
				fmt.Sprintf("Un-supported query param for pagination."+
					" Page size must be greater than 0. Invalid page"+
					" size value: %v", req.URL.Query().Get("pagesize")))
			return
		}
		if pageNoErr == nil && pageSizeErr == nil {
			startIndex, endIndex = Paginate(pageNo, pageSize, models.EVENTS_PER_PAGE)
		} else {
			logger.Get().Error("%s-Un-supported query param: %v, %v ", ctxt, pageNo, pageSize)
			HttpResponse(rw, http.StatusBadRequest, fmt.Sprintf("Un-supported query param for pagination: %v, %v", req.URL.Query().Get("pageno"), req.URL.Query().Get("pagesize")))
			return
		}
		if startIndex > len(events)-1 {
			logger.Get().Error("%s-Un-supported query param: %v, %v ", ctxt, pageNo, pageSize)
			HttpResponse(rw, http.StatusBadRequest, fmt.Sprintf("Un-supported query param for pagination: %v, %v", req.URL.Query().Get("pageno"), req.URL.Query().Get("pagesize")))
			return
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
	if err := collection.Find(bson.M{"eventid": *event_id}).One(&event); err == mgo.ErrNotFound {
		HttpResponse(w, http.StatusBadRequest, "Event not found")
		logger.Get().Error("%s-Event: %v not found. error: %v", ctxt, *event_id, err)
		return
	} else if err != nil {
		logger.Get().Error("%s-Error getting the event detail for %v. error: %v", ctxt, *event_id, err)
		HttpResponse(w, http.StatusBadRequest, "Event finding the record")
		return
	} else {
		json.NewEncoder(w).Encode(event)
	}
}

func PatchEvent(w http.ResponseWriter, r *http.Request) {
	ctxt, err := GetContext(r)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	var m map[string]interface{}
	vars := mux.Vars(r)
	event_id_str := vars["event-id"]
	event_id, err := uuid.Parse(event_id_str)
	if err != nil {
		logger.Get().Error("%s-Error parsing event id: %s", ctxt, event_id_str)
		HttpResponse(w, http.StatusBadRequest, fmt.Sprintf("Error parsing event id: %s", event_id_str))
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		HandleHttpError(w, err)
		return
	}
	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data:%s", ctxt, err)
		HandleHttpError(w, err)
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_APP_EVENTS)
	var event models.AppEvent
	if err := collection.Find(bson.M{"eventid": *event_id}).One(&event); err == mgo.ErrNotFound {
		HttpResponse(w, http.StatusBadRequest, "Event not found")
		logger.Get().Error("%s-Event: %v not found. error: %v", ctxt, *event_id, err)
		return
	} else if err != nil {
		logger.Get().Error("%s-Error getting the event detail for %v. error: %v", ctxt, *event_id, err)
		HttpResponse(w, http.StatusBadRequest, "Event finding the record")
		return
	}

	if event.Severity == models.ALARM_STATUS_CLEARED {
		logger.Get().Error("%s-Cannot ack an event with severity: %s", ctxt, event.Severity.String())
		HttpResponse(w, http.StatusBadRequest, "Event with this severity cannot be acked. ")
		return
	}

	if event.Acked {
		logger.Get().Error("%s-Cannot ack this event as its already acked", ctxt)
		HttpResponse(w, http.StatusBadRequest, "Event Cannot be acked as its already acked.")
		return
	}

	if val, ok := m["acked"]; ok {
		event.Acked = val.(bool)
	} else {
		logger.Get().Error("%s-Insufficient details for patching event: %v", ctxt, *event_id)
		HandleHttpError(w, errors.New("insufficient detail for patching event"))
		return
	}
	if val, ok := m["ackcomment"]; ok {
		event.UserAckComment = val.(string)
	}
	event.AckedByUser = strings.Split(ctxt, ":")[0]
	event.UserAckedTime = time.Now()

	err = collection.Update(bson.M{"eventid": *event_id}, bson.M{"$set": event})
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error updating record in DB for event: %v. error: %v", ctxt, event_id_str, err))
		HttpResponse(w, http.StatusInternalServerError, err.Error())
	}
	return
}
