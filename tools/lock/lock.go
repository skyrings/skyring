package lock

import (
	"sync"
)

type LockInternal struct {
	Mutex   sync.Mutex
	Message []string
}

func (l *LockInternal) AddMessage(message string) {
	l.Mutex.Lock()
	defer l.Mutex.Unlock()
	l.Message = append(l.Message, message)
}

func (l *LockInternal) GetMessages() (messages []string) {
	l.Mutex.Lock()
	defer l.Mutex.Unlock()
	return l.Message
}

func NewLockInternal(message string) *LockInternal {
	var messages []string
	messages = append(messages, message)
	return &LockInternal{Message: messages}
}
