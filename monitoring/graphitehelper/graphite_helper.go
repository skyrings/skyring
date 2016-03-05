package graphitehelper

import (
	"github.com/skyrings/skyring-common/monitoring"
	"github.com/skyrings/skyring-common/monitoring/graphitemanager"
	skyring_monitoring "github.com/skyrings/skyring/monitoring"
)

type GraphiteHelper struct {
}

const HelperManagerName = graphitemanager.TimeSeriesDBManagerName

var resourceToCollectionNameMapper = map[string]string{
	monitoring.NETWORK_LATENCY: "ping.ping-{{.serverName}}",
}

func init() {
	skyring_monitoring.RegisterMonitoringHelper(HelperManagerName, func() (skyring_monitoring.MonitoringHelperInterface, error) {
		return NewGraphiteHelper()
	})
}

func NewGraphiteHelper() (*GraphiteHelper, error) {
	return &GraphiteHelper{}, nil
}

func (gh GraphiteHelper) GetResourceToCollectionNameMapper() map[string]string {
	return resourceToCollectionNameMapper
}
