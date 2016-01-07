package influxdbmanager

import (
	influxdb "github.com/influxdb/influxdb/client"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
)

func queryDB(cmd string) (res []influxdb.Result, err error) {
	q := influxdb.Query{
		Command:  cmd,
		Database: conf.SystemConfig.TimeSeriesDBConfig.CollectionName,
	}

	if response, err := db.GetMonitoringDBClient().Query(q); err == nil {
		if response.Error() != nil {
			return res, response.Error()
		}
		res = response.Results
	}
	return
}

func QueryDB(params map[string]interface{}) (interface{}, error) {
	return nil, nil
}
