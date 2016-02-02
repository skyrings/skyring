package logger

import "github.com/op/go-logging"

func MockInit() {
	log = logging.MustGetLogger("Testing")
}
