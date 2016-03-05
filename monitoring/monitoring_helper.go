package monitoring

import (
	"fmt"
	"github.com/skyrings/skyring-common/tools/logger"
	"sync"
)

type MonitoringHelpersFactory func() (MonitoringHelperInterface, error)

var (
	monitoringHelpersMutex sync.Mutex
	monitoringHelpers      = make(map[string]MonitoringHelpersFactory)
)

func RegisterMonitoringHelper(name string, factory MonitoringHelpersFactory) {
	monitoringHelpersMutex.Lock()
	defer monitoringHelpersMutex.Unlock()

	if _, found := monitoringHelpers[name]; found {
		logger.Get().Info("Monitoring helper already registered. Returing..")
		return
	}
	monitoringHelpers[name] = factory
}

func GetMonitoringHelper(name string) (MonitoringHelperInterface, error) {
	monitoringHelpersMutex.Lock()
	defer monitoringHelpersMutex.Unlock()

	factory_func, found := monitoringHelpers[name]
	if !found {
		logger.Get().Info("Monitoring helper not found", name)
		return nil, nil
	}

	return factory_func()
}

func InitMonitoringHelper(name string) (MonitoringHelperInterface, error) {
	var helper MonitoringHelperInterface

	if name == "" {
		return nil, nil
	}

	var err error
	helper, err = GetMonitoringHelper(name)

	if err != nil {
		return nil, fmt.Errorf("Could not initialize monitoring helper %s: %v", name, err)
	}
	if helper == nil {
		return nil, fmt.Errorf("Unknown monitoring he lper %s", name)
	}

	return helper, nil
}
