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
	"errors"
	"fmt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"sync"
)

type Manager struct {
}

func (manager *Manager) Run(name string, f func(t *Task), startedFunc func(t *Task), completedFunc func(t *Task), statusFunc func(t *Task, s *models.Status)) (uuid.UUID, error) {
	if id, err := uuid.New(); err == nil {
		task := Task{
			Mutex:            &sync.Mutex{},
			ID:               *id,
			Name:             name,
			DoneCh:           make(chan bool, 1),
			StatusList:       []models.Status{},
			StopCh:           make(chan bool, 1),
			Func:             f,
			StartedCbkFunc:   startedFunc,
			CompletedCbkFunc: completedFunc,
			StatusCbkFunc:    statusFunc,
		}
		task.Run()
		return *id, nil
	} else {
		return uuid.UUID{}, err
	}
}

func (manager *Manager) IsDone(id uuid.UUID) (b bool, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var task models.AppTask
	if err := coll.Find(bson.M{"id": id}).One(&task); err != nil {
		err = errors.New(fmt.Sprintf("task id %s not found", id))
	} else {
		b = task.Completed
	}
	return
}

func (manager *Manager) IsStarted(id uuid.UUID) (b bool, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var task models.AppTask
	if err := coll.Find(bson.M{"id": id}).One(&task); err != nil {
		err = errors.New(fmt.Sprintf("task id %s not found", id))
	} else {
		b = task.Started
	}
	return
}

func (manager *Manager) GetStatus(id uuid.UUID) (status []models.Status, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var task models.AppTask
	if err := coll.Find(bson.M{"id": id}).One(&task); err != nil {
		err = errors.New(fmt.Sprintf("task id %s not found", id))
	} else {
		status = task.StatusList
	}
	return
}

func (manager *Manager) Remove(id uuid.UUID) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	_ = coll.Remove(bson.M{"id": id})
}

func (manager *Manager) List() []uuid.UUID {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_TASKS)
	var tasks []models.AppTask
	if err := coll.Find(nil).All(&tasks); err != nil {
		return []uuid.UUID{}
	}
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.Id)
	}
	return ids
}

func NewManager() Manager {
	return Manager{}
}
