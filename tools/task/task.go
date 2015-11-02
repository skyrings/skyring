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
	"github.com/skyrings/skyring/tools/uuid"
	"sync"
	"time"
)

type Status struct {
	Timestamp time.Time
	Message   string
}

func (s Status) String() string {
	return fmt.Sprintf("%s %s", s.Timestamp, s.Message)
}

type Task struct {
	Mutex     sync.Mutex
	ID        uuid.UUID
	Name      string
	Tag       map[string]string
	Started   bool
	Completed bool
	DoneCh    chan bool
	StatusList []Status
	StopCh    chan bool
	Func      func(t *Task)
}

func (t Task) String() string {
	return fmt.Sprintf("Task{ID=%s, Name=%s, Started=%t, Completed=%t}", t.ID, t.Name, t.Started, t.Completed)
}

func (t *Task) UpdateStatus(format string, args ...interface{}) {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	t.StatusList = append(t.StatusList, Status{time.Now(), fmt.Sprintf(format, args...)})
}

func (t *Task) GetStatus() (status []Status) {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	status = t.StatusList
	t.StatusList = []Status{}
	return
}

func (t *Task) Run() {
	go t.Func(t)
	t.Started = true
}

func (t *Task) Done() {
	t.DoneCh <- true
	close(t.DoneCh)
	t.Completed = true
}

func (t *Task) IsDone() bool {
	select {
	case _, read := <-t.DoneCh:
		if read == true {
			t.Completed = true
			return true
		} else {
			// DoneCh is in closed state
			return true
		}
	default:
		return false
	}
}
