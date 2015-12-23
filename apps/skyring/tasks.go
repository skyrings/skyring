package skyring

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/uuid"
	"github.com/skyrings/skyring/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
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
				logger.Get().Error("Unable to Parse the Id:%s", err)
				util.HandleHttpError(rw, err)
				return
			}
		} else {
			logger.Get().Error("Un-supported query param: %v", rootTask)
			util.HttpResponse(rw, http.StatusInternalServerError, fmt.Sprintf("Un-supported query param: %s", rootTask))
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
			util.HttpResponse(rw, http.StatusInternalServerError, fmt.Sprintf("Un-supported query param: %s", taskStatus))
			return
		}
	}
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var tasks []models.AppTask
	if err := coll.Find(filter).All(&tasks); err != nil {
		logger.Get().Error("Unable to get tasks: %v", err)
		util.HttpResponse(rw, http.StatusInternalServerError, err.Error())
		return

	}
	if len(tasks) == 0 {
		json.NewEncoder(rw).Encode([]models.AppTask{})
	} else {
		json.NewEncoder(rw).Encode(tasks)
	}
}

func (a *App) getTask(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	taskId, err := uuid.Parse(vars["taskid"])
	if err != nil {
		logger.Get().Error("Unable to Parse the Id:%s", err)
		util.HandleHttpError(rw, err)
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var task models.AppTask
	if err := coll.Find(bson.M{"id": *taskId}).One(&task); err != nil {
		logger.Get().Error("Unable to get task: %v", err)
		if err == mgo.ErrNotFound {
			util.HttpResponse(rw, http.StatusNotFound, err.Error())
			return
		} else {
			util.HttpResponse(rw, http.StatusInternalServerError, err.Error())
			return
		}
	}
	json.NewEncoder(rw).Encode(task)
}

func (a *App) getSubTasks(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	taskId, err := uuid.Parse(vars["taskid"])
	if err != nil {
		logger.Get().Error("Unable to Parse the Id:%s", err)
		util.HandleHttpError(rw, err)
		return
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var tasks []models.AppTask
	if err := coll.Find(bson.M{"parentid": *taskId}).All(&tasks); err != nil {
		logger.Get().Error("Unable to get tasks: %v", err)
		util.HttpResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if len(tasks) == 0 {
		json.NewEncoder(rw).Encode([]models.AppTask{})
	} else {
		json.NewEncoder(rw).Encode(tasks)
	}
}
