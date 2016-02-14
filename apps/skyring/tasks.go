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
)

const (
	rootTaskId = "00000000-0000-0000-0000-000000000000"
)

func (a *App) getTasks(rw http.ResponseWriter, req *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	var filter bson.M = make(map[string]interface{})

	rootTask := req.URL.Query().Get("level")
	taskStatus := req.URL.Query().Get("state")

	if len(rootTask) != 0 {
		if strings.ToLower(rootTask) == "root" {
			filter["parentid"], err = uuid.Parse(rootTaskId)
			if err != nil {
				logger.Get().Error("Unable to Parse the Id: %s. error: %v", rootTaskId, err)
				HandleHttpError(rw, err)
				return
			}
		} else {
			logger.Get().Error("Un-supported query param: %v", rootTask)
			HttpResponse(rw, http.StatusInternalServerError, fmt.Sprintf("Un-supported query param: %s", rootTask))
			return
		}
	}
	if len(taskStatus) != 0 {
		if strings.ToLower(taskStatus) == "inprogress" {
			filter["completed"] = false
		} else if strings.ToLower(taskStatus) == "completed" {
			filter["completed"] = true
		} else {
			logger.Get().Error("Un-supported query param: %v", taskStatus)
			HttpResponse(rw, http.StatusInternalServerError, fmt.Sprintf("Un-supported query param: %s", taskStatus))
			return
		}
	}
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var tasks []models.AppTask
	pageNo, pageNoErr := strconv.Atoi(req.URL.Query().Get("pageNo"))
	pageSize, pageSizeErr := strconv.Atoi(req.URL.Query().Get("pageSize"))

	if err := coll.Find(filter).All(&tasks); err != nil {
		logger.Get().Error("Unable to get tasks. error: %v", err)
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
	vars := mux.Vars(req)
	taskId, err := uuid.Parse(vars["taskid"])
	if err != nil {
		logger.Get().Error("Unable to Parse the Id: %s. error: %v", vars["taskId"], err)
		HandleHttpError(rw, err)
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var task models.AppTask
	if err := coll.Find(bson.M{"id": *taskId}).One(&task); err != nil {
		logger.Get().Error("Unable to get task. error: %v", err)
		if err == mgo.ErrNotFound {
			HttpResponse(rw, http.StatusNotFound, err.Error())
			return
		} else {
			HttpResponse(rw, http.StatusInternalServerError, err.Error())
			return
		}
	}
	json.NewEncoder(rw).Encode(task)
}

func (a *App) getSubTasks(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	taskId, err := uuid.Parse(vars["taskid"])
	if err != nil {
		logger.Get().Error("Unable to Parse the Id: %s. error: %v", vars["taskId"], err)
		HandleHttpError(rw, err)
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var tasks []models.AppTask
	if err := coll.Find(bson.M{"parentid": *taskId}).All(&tasks); err != nil {
		logger.Get().Error("Unable to get tasks. error: %v", err)
		HttpResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if len(tasks) == 0 {
		json.NewEncoder(rw).Encode([]models.AppTask{})
	} else {
		json.NewEncoder(rw).Encode(tasks)
	}
}
