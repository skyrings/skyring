package schedule

import (
	"github.com/skyrings/skyring/tools/uuid"
	"time"
)

type Scheduler struct {
	Id      uuid.UUID
	Channel chan string
}

var (
	scheduler *Scheduler
)

func (s Scheduler) Schedule(t time.Duration, f func(map[string]interface{}), m map[string]interface{}) {
	for {
		select {
		case _, ok := <-time.After(t):
			if ok {
				f(m)
			}
		case _, ok := <-s.Channel:
			if ok {
				return
			}
		}
	}
}
