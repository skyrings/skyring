package log

import (
	"github.com/op/go-logging"
	"os"
	"path"
)

var fileFormat = logging.MustStringFormatter(
	"%{time} %{level: -8s} %{shortfile} %{shortfunc}] %{message}",
)
var stderrFormat = logging.MustStringFormatter(
	"%{color}%{time} %{level: -8s} %{shortfile} %{shortfunc}]%{color:reset} %{message}",
)

var logger *logging.Logger
var loggerInit = false

func Init(filename string, logToStderr bool, level logging.Level) error {
	if loggerInit {
		return nil
	}

	var fileBackend, stderrBackend *logging.LogBackend
	var fileLeveled, stderrLeveled logging.LeveledBackend

	logger = logging.MustGetLogger(path.Base(os.Args[0]))

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
		logger.SetBackend(logging.MultiLogger(stderrLeveled, fileLeveled))
	} else {
		logger.SetBackend(fileLeveled)
	}

	loggerInit = true
	return nil
}

func Get() *logging.Logger {
	return logger
}
