package schedule

import (
	"github.com/skyrings/skyring/tools/uuid"
)

//In memory Schedule id to Schedule Map
var schedules map[uuid.UUID]Scheduler

func NewScheduler() (Scheduler, error) {
	id, err := uuid.New()
	if err != nil {
		return Scheduler{}, err
	}
	scheduler := Scheduler{Channel: make(chan string), Id: *id}
	if schedules == nil {
		schedules = make(map[uuid.UUID]Scheduler)
	}
	schedules[*id] = scheduler
	return scheduler, nil
}

func DeleteScheduler(scheduleId uuid.UUID) {
	go func() {
		scheduler.Channel <- "Done"
		delete(schedules, scheduleId)
	}()
}

func GetScheduler(scheduleId uuid.UUID) (Scheduler, error) {
	for scheduledId := range schedules {
		if uuid.Equal(scheduledId, scheduleId) {
			return schedules[scheduleId], nil
		}
	}
	return schedules[scheduleId], nil
}
