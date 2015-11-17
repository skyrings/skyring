/*Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package lock

import (
	"errors"
	"fmt"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/uuid"
	"sync"
)

type Manager struct {
	/*
	   TODO: This part will move into the DB to operate between core
	   and multiple providers
	*/
	locks map[uuid.UUID]*LockInternal
}

type LockManager interface {
	//The following method will try to acquire provided lock
	AcquireLock(id uuid.UUID, lock *LockInternal) error
	// The following method will release a lock
	ReleaseLock(id uuid.UUID) error
	//The following method will clear all inserted locks
	Clear() error
}

var lockMutex sync.Mutex

func NewLockManager() *Manager {
	return &Manager{make(map[uuid.UUID]*LockInternal)}
}

func (manager *Manager) AcquireLock(appLock AppLock) error {
	lockMutex.Lock()
	defer lockMutex.Unlock()
	for k, v := range appLock.GetAppLocks() {
		//check if the lock exists
		if val, ok := manager.locks[k]; ok {
			//Lock already aquired return from here
			err := fmt.Sprintf("Unable to Acquire the lock for %v Message %s ", k, val.GetMessages())
			logger.Get().Error("Unable to Acquire the lock for: ", k)
			return errors.New(err)
		}
		logger.Get().Error("Lock Acquired for: ", k)
		manager.locks[k] = NewLockInternal(v)
	}
	return nil

}

func (manager *Manager) ReleaseLock(appLock AppLock) error {
	lockMutex.Lock()
	defer lockMutex.Unlock()
	for k := range appLock.GetAppLocks() {
		//check if the lock exists
		if _, ok := manager.locks[k]; !ok {
			//No lock exits log and do nothing
			err := fmt.Sprintf("No Lock found for unlocking %v", k)
			logger.Get().Error("No Lock found for unlocking: ", k)
			return errors.New(err)
		}
		logger.Get().Error("Lock Released: ", k)
		delete(manager.locks, k)
	}
	return nil
}

func (manager *Manager) Clear() error {
	lockMutex.Lock()
	defer lockMutex.Unlock()
	for k := range manager.locks {
		delete(manager.locks, k)
	}
	return nil
}
