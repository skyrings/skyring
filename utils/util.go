package util

import (
	"encoding/json"
	"fmt"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/task"
	"net/http"
	"runtime"
	"time"
)

type APIError struct {
	Error string
}

// For testing, bypass HandleCrash.
var ReallyCrash bool

// PanicHandlers is a list of functions which will be invoked when a panic happens.
var PanicHandlers = []func(interface{}){logPanic}

// HandleCrash simply catches a crash and logs an error. Meant to be called via defer.
func HandleCrash() {
	if ReallyCrash {
		return
	}
	if r := recover(); r != nil {
		for _, fn := range PanicHandlers {
			fn(r)
		}
	}
}

// Deprecated. Please use Until and pass NeverStop as the stopCh.
func Forever(f func(), period time.Duration) {
	Until(f, period, nil)
}

// Until loops until stop channel is closed, running f every period.
// Catches any panics, and keeps going. f may not be invoked if
// stop channel is already closed.
func Until(f func(), period time.Duration, stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			return
		default:
		}
		func() {
			defer HandleCrash()
			f()
		}()
		time.Sleep(period)
	}
}

// logPanic logs the caller tree when a panic occurs.
func logPanic(r interface{}) {
	callers := ""
	for i := 0; true; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		callers = callers + fmt.Sprintf("%v:%v\n", file, line)
	}
	logger.Get().Error("Recovered from panic: %#v (%v)\n%v", r, r, callers)
}

func HandleHttpError(rw http.ResponseWriter, err error) {
	bytes, _ := json.Marshal(APIError{Error: err.Error()})
	rw.WriteHeader(http.StatusInternalServerError)
	rw.Write(bytes)
}

func HttpResponse(w http.ResponseWriter, status_code int, msg string) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.WriteHeader(status_code)
	if err := json.NewEncoder(w).Encode(msg); err != nil {
		logger.Get().Error("Error: %v", err)
	}
	return
}

func FailTask(msg string, err error, t *task.Task) {
	logger.Get().Error("%s: %v", msg, err)
	t.UpdateStatus("Failed. error: %v", err)
	t.Done(models.TASK_STATUS_FAILURE)
}
