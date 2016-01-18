package influxdbmanager

import (
	"fmt"
	influxdb "github.com/influxdb/influxdb/client"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/utils"
	"io"
	"regexp"
	"time"
)

const (
	TimeSeriesDBManagerName = "InfluxdbManager"
)

type InfluxdbManager struct {
}

func init() {
	monitoring.RegisterMonitoringManager(TimeSeriesDBManagerName, func(config io.Reader) (monitoring.MonitoringManagerInterface, error) {
		return NewInfluxdbManager(config)
	})
}

func NewInfluxdbManager(config io.Reader) (*InfluxdbManager, error) {
	return &InfluxdbManager{}, nil
}

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

func (idm InfluxdbManager) QueryDB(params map[string]interface{}) (interface{}, error) {
	var resource_name string
	var nodename string
	var startTime string
	var endTime string
	var interval string
	var err error

	if resource_name, err = util.GetString(params["resource"]); err != nil {
		return nil, err
	}
	if nodename, err = util.GetString(params["nodename"]); err != nil {
		return nil, err
	}
	if startTime, err = util.GetString(params["start_time"]); err != nil {
		return nil, err
	}
	if endTime, err = util.GetString(params["end_time"]); err != nil {
		return nil, err
	}
	if interval, err = util.GetString(params["interval"]); err != nil {
		return nil, err
	}

	var query_cmd string
	if resource_name != "" {
		query_cmd = fmt.Sprintf("SELECT * FROM /(%s.%s).*/", nodename, resource_name)
	} else {
		query_cmd = fmt.Sprintf("SELECT * FROM /(%s).*/", nodename)
	}

	if startTime != "" && endTime != "" {
		if _, err := time.Parse("2006-01-02T15:04:05.000Z", startTime); err != nil {
			return nil, fmt.Errorf("Error parsing start time: %s", startTime)
		}
		if _, err := time.Parse("2006-01-02T15:04:05.000Z", endTime); err != nil {
			return nil, fmt.Errorf("Error parsing end time: %s", endTime)
		}
		query_cmd += " WHERE time > '" + startTime + "' and time < '" + endTime + "'"
	}

	if interval != "" {
		if matched, _ := regexp.Match("^([0-5]?[0-9])?s$|^([0-5]?[0-9])?m$|^([0-2]?[0-3])?h$|^([0-9])*d$|^([0-9])*w$", []byte(interval)); !matched {
			return nil, fmt.Errorf("Invalid duration passed: %s", interval)
		}
		query_cmd += " WHERE time > now() - " + interval
	}

	res, err := queryDB(query_cmd)
	return res, err
}

func PushToDb(metrics interface{}) error {
	/*
		TODO Implement
	*/
	return nil
}
