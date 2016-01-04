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
	"time"
)

type Scheduler struct {
	ch          chan string
	taskManager Manager
}

var (
	scheduler *Scheduler
)

func GetScheduler(manager Manager) *Scheduler {
	if scheduler == nil {
		tScheduler := Scheduler{make(chan string), manager}
		scheduler = &tScheduler
	}
	return scheduler
}

func (s *Scheduler) Schedule(t time.Duration, f func(), args ...interface{}) {
	fmt.Println("Entered schedule")
	for {
		select {
		case _, ok := <-time.After(t):
			if ok {
				//s.taskManager.Run("sync_task", f, nil, nil, nil)
				fmt.Println("!!!!Triggering the schedule!!!!")
				f()
			}
		}
	}
}
