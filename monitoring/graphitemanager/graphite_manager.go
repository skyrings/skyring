package graphitemanager

import (
	"github.com/skyrings/skyring/monitoring"
	"io"
)
const (
	TimeSeriesDBManagerName = "GraphiteManager"
)

type GraphiteManager struct {
}

func init() {
	monitoring.RegisterNodeManager(TimeSeriesDBManagerName, func(config io.Reader) (monitoring.MonitoringManagerInterface, error) {
		return NewGraphiteManager(config)
	})
}

func NewGraphiteManager(config io.Reader) (*GraphiteManager, error) {
	return &GraphiteManager{}, nil
}

func (tsdbm GraphiteManager) QueryDB(resource, time string) (interface{}, error) {

}
