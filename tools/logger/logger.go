package logger

import (
	"errors"
	"github.com/op/go-logging"
	"github.com/skyrings/skyring/tools/gopy"
	"os"
	"path"
)

var fileFormat = logging.MustStringFormatter(
	"%{time} %{level: -8s} %{shortfile} %{shortfunc}] %{message}",
)
var stderrFormat = logging.MustStringFormatter(
	"%{color}%{time} %{level: -8s} %{shortfile} %{shortfunc}]%{color:reset} %{message}",
)

var log *logging.Logger
var logInit = false

func Init(filename string, logToStderr bool, level logging.Level) error {
	if logInit {
		return nil
	}

	logName := path.Base(os.Args[0])

	if pyFunc, err := gopy.Import("skyring", "InitLog"); err != nil {
		return err
	} else if pyobj, err := pyFunc["InitLog"].Call(logName, filename, logToStderr, level.String()); err != nil {
		return err
	} else if !gopy.Bool(pyobj) {
		return errors.New("python: log init failed")
	}

	var fileBackend, stderrBackend *logging.LogBackend
	var fileLeveled, stderrLeveled logging.LeveledBackend

	log = logging.MustGetLogger(logName)

	if file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err != nil {
		return err
	} else {
		fileBackend = logging.NewLogBackend(file, "", 0)
		fileLeveled = logging.AddModuleLevel(logging.NewBackendFormatter(fileBackend, fileFormat))
		fileLeveled.SetLevel(level, "")
	}

	if logToStderr {
		stderrBackend = logging.NewLogBackend(os.Stderr, "", 0)
		stderrLeveled = logging.AddModuleLevel(logging.NewBackendFormatter(stderrBackend, stderrFormat))
		stderrLeveled.SetLevel(level, "")
		log.SetBackend(logging.MultiLogger(stderrLeveled, fileLeveled))
	} else {
		log.SetBackend(fileLeveled)
	}

	logInit = true
	return nil
}

func Get() *logging.Logger {
	return log
}
