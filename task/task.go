package task

import (
	"errors"
	"fmt"
	"github.com/skyrings/skyring/uuid"
	"reflect"
	"runtime"
)

type Task struct {
	id        *uuid.UUID
	started   bool
	completed bool
	done      chan bool
}

func (t *Task) String() string {
	return fmt.Sprintf("Task(id=%s, started=%t, completed=%t)", t.id, t.started, t.completed)
}

func (t *Task) Done() bool {
	select {
	case _, read := <-t.done:
		if read == true {
			t.completed = true
			return true
		} else {
			fmt.Println(t, "done channel in closed state")
			return true
		}
	default:
		return false
	}
}

type TaskManager struct {
	tasks map[uuid.UUID]*Task
}

func (manager *TaskManager) Run(function interface{}, args ...interface{}) (*uuid.UUID, error) {
	f := reflect.ValueOf(function)
	if f.Kind() != reflect.Func {
		return nil, errors.New(fmt.Sprintf("`%s` is not callable interface", function))
	}

	fname := runtime.FuncForPC(f.Pointer()).Name()
	num_in := f.Type().NumIn()
	if num_in != len(args) {
		return nil, errors.New(fmt.Sprintf("insufficient arguments to %s %T", fname, function))
	}
	num_out := f.Type().NumOut()

	// fmt.Printf("function name: %s()\n", fname)
	// fmt.Printf("\targs count: %d\n", num_in)
	// fmt.Printf("\treturn values count: %d\n",num_out)

	ins := make([]reflect.Value, num_in)
	for k, arg := range args {
		ins[k] = reflect.ValueOf(arg)
	}

	id := uuid.New()
	task := &Task{id: id, done: make(chan bool, 1)}
	manager.tasks[*id] = task
	go func() {
		task.started = true
		result := f.Call(ins)

		if num_out > 0 {
			for i, r := range result {
				fmt.Printf("task-id[%d] returns result[%d]: %s\n", task.id, i, r)
			}
		}

		task.done <- true
		close(task.done)
	}()

	rv := *id
	return &rv, nil
}

func (manager *TaskManager) Done(id uuid.UUID) (bool, error) {
	if task, ok := manager.tasks[id]; ok {
		if task.Done() {
			delete(manager.tasks, id)
			return true, nil
		} else {
			return false, nil
		}
	} else {
		return false, errors.New(fmt.Sprintf("task-id %s not found", &id))
	}
}

func (manager *TaskManager) List() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(manager.tasks))
	for k := range manager.tasks {
		ids = append(ids, k)
	}

	return ids
}

func NewTaskManager() TaskManager {
	return TaskManager{make(map[uuid.UUID]*Task)}
}
