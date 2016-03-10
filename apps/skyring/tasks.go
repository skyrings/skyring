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
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	rootTaskId = "00000000-0000-0000-0000-000000000000"
)

func (a *App) getTasks(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	var filter bson.M = make(map[string]interface{})

	rootTask := req.URL.Query().Get("level")
	taskStatusArray := req.URL.Query()["state"]
	searchMessage := req.URL.Query().Get("searchmessage")

	if len(rootTask) != 0 {
		if strings.ToLower(rootTask) == "root" {
			filter["parentid"], err = uuid.Parse(rootTaskId)
			if err != nil {
				logger.Get().Error("%s-Unable to Parse the Id: %s. error: %v", ctxt, rootTaskId, err)
				HandleHttpError(rw, err)
				return
			}
		} else {
			logger.Get().Error("%s-Un-supported query param: %v", ctxt, rootTask)
			HttpResponse(rw, http.StatusInternalServerError, fmt.Sprintf("Un-supported query param: %s", rootTask))
			return
		}
	}

	taskStatusMap := make(map[string]string)
	for _, taskStatus := range taskStatusArray {
		taskStatusMap[strings.ToLower(taskStatus)] = taskStatus
	}

	if len(taskStatusMap) != 0 {
		_, inprogressErr := taskStatusMap["inprogress"]
		_, completedErr := taskStatusMap["completed"]
		_, failedErr := taskStatusMap["failed"]

		if inprogressErr && !completedErr {
			filter["completed"] = false
		}
		if !inprogressErr && completedErr {
			filter["completed"] = true
		}
		if failedErr {
			filter["status"] = models.TASK_STATUS_FAILURE
		}
	}
	if len(searchMessage) != 0 {
		filter["name"] = bson.M{"$regex": searchMessage, "$options": "$i"}
	}
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var tasks []models.AppTask
	pageNo, pageNoErr := strconv.Atoi(req.URL.Query().Get("pageNo"))
	pageSize, pageSizeErr := strconv.Atoi(req.URL.Query().Get("pageSize"))
	// time stamp format of RFC3339 = "2006-01-02T15:04:05Z07:00"
	fromDateTime, fromDateTimeErr := time.Parse(time.RFC3339, req.URL.Query().Get("fromdatetime"))
	toDateTime, toDateTimeErr := time.Parse(time.RFC3339, req.URL.Query().Get("todatetime"))
	if fromDateTimeErr == nil && toDateTimeErr == nil {
		filter["lastupdated"] = bson.M{
			"$gt": fromDateTime.UTC(),
			"$lt": toDateTime.UTC(),
		}
	} else if fromDateTimeErr != nil && toDateTimeErr == nil {
		filter["lastupdated"] = bson.M{
			"$lt": toDateTime.UTC(),
		}
	} else if fromDateTimeErr == nil && toDateTimeErr != nil {
		filter["lastupdated"] = bson.M{
			"$gt": fromDateTime.UTC(),
		}
	}
	if err := coll.Find(filter).Sort("-completed", "lastupdated").All(&tasks); err != nil {
		logger.Get().Error("%s-Unable to get tasks. error: %v", ctxt, err)
		HttpResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}

	if len(tasks) == 0 {
		json.NewEncoder(rw).Encode([]models.AppTask{})
	} else {
		var startIndex int = 0
		var endIndex int = models.TASKS_PER_PAGE
		if pageNoErr == nil && pageSizeErr == nil {
			startIndex, endIndex = Paginate(pageNo, pageSize, models.TASKS_PER_PAGE)
		}
		if endIndex > len(tasks) {
			endIndex = len(tasks) - 1
		}

		json.NewEncoder(rw).Encode(
			struct {
				Totalcount int              `json:"totalcount"`
				Startindex int              `json:"startindex"`
				Endindex   int              `json:"endindex"`
				Tasks      []models.AppTask `json:"tasks"`
			}{len(tasks), startIndex, endIndex, tasks[startIndex : endIndex+1]})

	}
}

func (a *App) getTask(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(req)
	taskId, err := uuid.Parse(vars["taskid"])
	if err != nil {
		logger.Get().Error("%s-Unable to Parse the Id: %s. error: %v", ctxt, vars["taskId"], err)
		HandleHttpError(rw, err)
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var task models.AppTask
	if err := coll.Find(bson.M{"id": *taskId}).One(&task); err != nil {
		logger.Get().Error("%s-Unable to get task. error: %v", ctxt, err)
		if err == mgo.ErrNotFound {
			HttpResponse(rw, http.StatusNotFound, err.Error())
			return
		} else {
			logger.Get().Error("%s-Error getting the task details for %v. error: %v", ctxt, *taskId, err)
			HttpResponse(rw, http.StatusInternalServerError, err.Error())
			return
		}
	}
	json.NewEncoder(rw).Encode(task)
}

func (a *App) getSubTasks(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(req)
	taskId, err := uuid.Parse(vars["taskid"])
	if err != nil {
		logger.Get().Error("%s-Unable to Parse the Id: %s. error: %v", ctxt, vars["taskId"], err)
		HandleHttpError(rw, err)
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var tasks []models.AppTask
	if err := coll.Find(bson.M{"parentid": *taskId}).All(&tasks); err != nil {
		logger.Get().Error("%s-Unable to get tasks. error: %v", ctxt, err)
		HttpResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if len(tasks) == 0 {
		json.NewEncoder(rw).Encode([]models.AppTask{})
	} else {
		json.NewEncoder(rw).Encode(tasks)
	}
}
