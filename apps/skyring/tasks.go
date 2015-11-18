package skyring

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/task"
	"github.com/skyrings/skyring/tools/uuid"
	"github.com/skyrings/skyring/utils"
	"gopkg.in/mgo.v2/bson"
	"net/http"
)

func (a *App) getTasks(rw http.ResponseWriter, req *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var tasks []task.AppTask
	if err := coll.Find(nil).All(&tasks); err != nil {
		util.HttpResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if len(tasks) == 0 {
		json.NewEncoder(rw).Encode([]task.AppTask{})
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
	var task task.AppTask
	if err := coll.Find(bson.M{"id": *taskId}).One(&task); err != nil {
		util.HttpResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if task.Id.IsZero() {
		util.HttpResponse(rw, http.StatusBadRequest, "Task not found")
		logger.Get().Error("Task not found: %v", err)
		return
	} else {
		json.NewEncoder(rw).Encode(task)
	}

}
