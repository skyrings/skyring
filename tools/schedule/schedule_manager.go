package schedule

import (
	"fmt"
	"github.com/skyrings/skyring/tools/uuid"
)

//In memory Schedule id to Schedule Map
var schedules map[uuid.UUID]Scheduler

func InitShechuleManager() {
	if schedules == nil {
		schedules = make(map[uuid.UUID]Scheduler)
	}
}
func NewScheduler() (Scheduler, error) {
	id, err := uuid.New()
	if err != nil {
		return Scheduler{}, err
	}
	scheduler := Scheduler{Channel: make(chan string), Id: *id}
	schedules[*id] = scheduler
	return scheduler, nil
}

func DeleteScheduler(scheduleId uuid.UUID) error {
	scheduler, err := GetScheduler(scheduleId)
	if err != nil {
		return err
	}
	go func() {
		scheduler.Channel <- "Done"
		delete(schedules, scheduleId)
	}()
	return nil
}

func GetScheduler(scheduleId uuid.UUID) (Scheduler, error) {
	val, ok := schedules[scheduleId]
	if ok {
		return val, nil
	}
	return val, fmt.Errorf("Schedule with id %v not found", scheduleId)
}
