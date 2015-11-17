package lock

import (
	"github.com/skyrings/skyring/tools/uuid"
)

type AppLock struct {
	locks map[uuid.UUID]string
}

func (a *AppLock) GetAppLocks() map[uuid.UUID]string {
	return a.locks
}

func NewAppLock(locks map[uuid.UUID]string) *AppLock {
	return &AppLock{locks}
}
