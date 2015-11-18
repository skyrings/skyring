// Copyright 2015 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package task

import (
	"fmt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"sync"
	"time"
)

type Task struct {
	Mutex            *sync.Mutex
	ID               uuid.UUID
	Name             string
	Tag              map[string]string
	Started          bool
	Completed        bool
	DoneCh           chan bool
	StatusList       []models.Status
	StopCh           chan bool
	Func             func(t *Task)
	StartedCbkFunc   func(t *Task)
	CompletedCbkFunc func(t *Task)
	StatusCbkFunc    func(t *Task, s *models.Status)
}

func (t Task) String() string {
	return fmt.Sprintf("Task{ID=%s, Name=%s, Started=%t, Completed=%t}", t.ID, t.Name, t.Started, t.Completed)
}

func (t *Task) UpdateStatus(format string, args ...interface{}) {
	s := models.Status{Timestamp: time.Now(), Message: fmt.Sprintf(format, args...)}
	t.Mutex.Lock()
	t.StatusList = append(t.StatusList, s)
	t.UpdateStatusList(t.StatusList)
	t.Mutex.Unlock()
	if t.StatusCbkFunc != nil {
		go t.StatusCbkFunc(t, &s)
	}
}

func (t *Task) GetStatus() (status []models.Status) {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	status = t.StatusList
	t.StatusList = []models.Status{}
	return
}

func (t *Task) Run() {
	go t.Func(t)
	t.Started = true
	t.Persist()
	if t.StartedCbkFunc != nil {
		go t.StartedCbkFunc(t)
	}
}

func (t *Task) Done() {
	t.DoneCh <- true
	close(t.DoneCh)
	t.Completed = true
	t.UpdateTaskCompleted(t.Completed)
}

func (t *Task) IsDone() bool {
	select {
	case _, read := <-t.DoneCh:
		if read == true {
			t.Completed = true
			if t.CompletedCbkFunc != nil {
				go t.CompletedCbkFunc(t)
			}
			return true
		} else {
			// DoneCh is in closed state
			return true
		}
	default:
		return false
	}
}

func (t *Task) Persist() (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)

	// Populate the task details. The parent ID should always be updated by the parent task later.
	var appTask models.AppTask
	appTask.Id = t.ID
	appTask.Started = t.Started
	appTask.Completed = t.Completed
	appTask.StatusList = t.StatusList

	if err := coll.Insert(appTask); err != nil {
		return false, err
	}

	return true, nil
}

func (t *Task) UpdateStatusList(status []models.Status) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	if err := coll.Update(bson.M{"id": t.ID}, bson.M{"$set": bson.M{"statuslist": status}}); err != nil {
		return false, err
	}

	return true, nil
}

func (t *Task) UpdateTaskCompleted(b bool) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	if err := coll.Update(bson.M{"id": t.ID}, bson.M{"$set": bson.M{"completed": b}}); err != nil {
		return false, err
	}

	return true, nil
}
