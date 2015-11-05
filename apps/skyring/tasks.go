package skyring

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/uuid"
	"github.com/skyrings/skyring/utils"
	"net/http"
)

func (a *App) getTasks(rw http.ResponseWriter, req *http.Request) {
	var taskList []models.Task
	tasks := a.GetTaskManager().List()
	for _, taskId := range tasks {
		started, _ := a.GetTaskManager().IsStarted(taskId)
		completed, _ := a.GetTaskManager().IsDone(taskId)
		statusList, _ := a.GetTaskManager().GetStatus(taskId)
		taskInfo := models.Task{
			Id:         taskId,
			Started:    started,
			Completed:  completed,
			StatusList: statusList}
		taskList = append(taskList, taskInfo)
	}
	//marshal and send it across
	bytes, err := json.Marshal(taskList)
	if err != nil {
		logger.Get().Error("Unable to marshal the list of Tasks:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)
}

func (a *App) getTask(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	taskId, err := uuid.Parse(vars["taskid"])
	if err != nil {
		logger.Get().Error("Unable to Parse the Id:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	started, _ := a.GetTaskManager().IsStarted(*taskId)
	completed, _ := a.GetTaskManager().IsDone(*taskId)
	statusList, _ := a.GetTaskManager().GetStatus(*taskId)
	taskInfo := models.Task{
		Id:         *taskId,
		Started:    started,
		Completed:  completed,
		StatusList: statusList}
	//marshal and send it across
	bytes, err := json.Marshal(taskInfo)
	if err != nil {
		logger.Get().Error("Unable to marshal the Task Info:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}
