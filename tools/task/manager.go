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
	"github.com/skyrings/skyring/tools/uuid"
	"sync"
)

type Manager struct {
	tasks map[uuid.UUID]*Task
}

func (manager *Manager) Run(name string, f func(t *Task), startedFunc func(t *Task), completedFunc func(t *Task), statusFunc func(t *Task, s *Status)) (uuid.UUID, error) {
	if id, err := uuid.New(); err == nil {
		task := Task{
			Mutex:            &sync.Mutex{},
			ID:               *id,
			Name:             name,
			DoneCh:           make(chan bool, 1),
			StatusList:       []Status{},
			StopCh:           make(chan bool, 1),
			Func:             f,
			StartedCbkFunc:   startedFunc,
			CompletedCbkFunc: completedFunc,
			StatusCbkFunc:    statusFunc
		}
		task.Run()
		manager.tasks[*id] = &task
		return *id, nil
	} else {
		return uuid.UUID{}, err
	}
}

func (manager *Manager) IsDone(id uuid.UUID) (b bool, err error) {
	if task, ok := manager.tasks[id]; ok {
		b = task.IsDone()
	} else {
		err = errors.New(fmt.Sprintf("task id %s not found", id))
	}
	return
}

func (manager *Manager) GetStatus(id uuid.UUID) (status []Status, err error) {
	if task, ok := manager.tasks[id]; ok {
		status = task.GetStatus()
	} else {
		err = errors.New(fmt.Sprintf("task id %s not found", id))
	}
	return
}

func (manager *Manager) Remove(id uuid.UUID) {
	delete(manager.tasks, id)
}

func (manager *Manager) List() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(manager.tasks))
	for k := range manager.tasks {
		ids = append(ids, k)
	}

	return ids
}

func NewManager() Manager {
	return Manager{make(map[uuid.UUID]*Task)}
}
