package graphitemanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/utils"
	"io"
	"regexp"
	"strings"
	"text/template"
	"time"
)

const (
	TimeSeriesDBManagerName = "GraphiteManager"
)

type GraphiteManager struct {
}

func init() {
	monitoring.RegisterMonitoringManager(TimeSeriesDBManagerName, func(config io.Reader) (monitoring.MonitoringManagerInterface, error) {
		return NewGraphiteManager(config)
	})
}

func NewGraphiteManager(config io.Reader) (*GraphiteManager, error) {
	return &GraphiteManager{}, nil
}

var GeneralResources = []string{
	"df",
	"memory",
	"cpu",
}

var (
	templateUrl        = "http://{{.hostname}}:{{.port}}/render?target={{.collectionname}}.{{.nodename}}.{{.resourcename}}*.*&format=json"
	templateFromTime   = "&from={{.start_time}}"
	templateUntilTime  = "&until={{.end_time}}"
	timeFormat         = "15:04_20060102"
	standardTimeFormat = "2006-01-02T15:04:05.000Z"
)

var SupportedInputTimeFormats = []string{
	"2006-01-02T15:04:05.000Z",
	"2006-01-02",
	"20060102",
}

func formatDate(inDate string) (string, error) {
	for _, currentFormat := range SupportedInputTimeFormats {
		if parsedTime, err := time.Parse(currentFormat, inDate); err == nil {
			return parsedTime.Format("15:04") + "_" + parsedTime.Format("20060102"), nil
		}
	}
	if matched, _ := regexp.Match("^-([0-5]?[0-9])?s$|^-([0-5]?[0-9])?min$|^-([0-2]?[0-3])?h$|^-([0-9])*d$|^-([0-9])*w$|^-([0-9])*mon$|^-([0-9])*y$", []byte(inDate)); !matched {
		return "", fmt.Errorf("Unsupported type")
	}
	return inDate, nil
}

func GetTemplateParsedString(urlParams map[string]interface{}, templateString string) (string, error) {
	buf := new(bytes.Buffer)
	parsedTemplate, err := template.New("graphite_url_parser").Parse(templateString)
	if err != nil {
		return "", err
	}
	parsedTemplate.Execute(buf, urlParams)
	return buf.String(), nil
}

func getUrlBaseTemplateParams(params map[string]interface{}) (map[string]interface{}, error) {
	var nodename string
	var nodeNameError error
	if nodename, nodeNameError = util.GetString(params["nodename"]); nodeNameError != nil {
		return nil, nodeNameError
	}
	nodename = strings.Replace(nodename, ".", "_", -1)
	return map[string]interface{}{
		"hostname":       conf.SystemConfig.TimeSeriesDBConfig.Hostname,
		"port":           conf.SystemConfig.TimeSeriesDBConfig.Port,
		"collectionname": conf.SystemConfig.TimeSeriesDBConfig.CollectionName,
		"nodename":       nodename,
		"resourcename":   params["resource"],
	}, nil
}

func Matches(key string, keys []string) bool {
	for _, permittedKey := range keys {
		if strings.Index(key, permittedKey) == 0 {
			return true
		}
	}
	return false
}

func (tsdbm GraphiteManager) QueryDB(params map[string]interface{}) (interface{}, error) {
	var resource string
	var data []interface{}
	if str, ok := params["resource"].(string); ok {
		resource = str
	}
	if !Matches(resource, GeneralResources) {
		/*
			1. Ideally fetch clusterId from nodeId
			2. Fetch clustertype from clusterId
			3. Based on clusterType, delegate the querying task to appropriate provider -- bigfin etc..
				i.  The provider will check internally if its monitoring provider supports the requested resource
					a. If it supports, get the results from it
					b. Else receive the error and propogate it.
			4. Return results
		*/
		return nil, fmt.Errorf("Unsupported Resource")
	} else {
		urlParams, urlParamsError := getUrlBaseTemplateParams(params)
		if urlParamsError != nil {
			return nil, urlParamsError
		}
		url, urlError := GetTemplateParsedString(urlParams, templateUrl)
		if urlError != nil {
			return nil, urlError
		}

		if params["start_time"] != "" {
			timeString, timeStringError := util.GetString(params["start_time"])
			if timeStringError != nil {
				return nil, fmt.Errorf("Start time %v. Error: %v", params["start_time"], timeStringError)
			}
			dateFormat, dateFormatErr := formatDate(timeString)
			if dateFormatErr != nil {
				return nil, dateFormatErr
			}
			params["start_time"] = dateFormat
			urlTime, urlTimeError := GetTemplateParsedString(params, templateFromTime)
			if urlTimeError != nil {
				return nil, urlTimeError
			}
			url = url + urlTime
		}

		if params["end_time"] != "" {
			timeString, timeStringError := util.GetString(params["end_time"])
			if timeStringError != nil {
				return nil, fmt.Errorf("End time %v. Error : %v", params["end_time"], timeStringError)
			}
			dateFormat, dateFormatErr := formatDate(timeString)
			if dateFormatErr != nil {
				return nil, dateFormatErr
			}
			params["end_time"] = dateFormat
			urlTime, urlTimeError := GetTemplateParsedString(params, templateUntilTime)
			if urlTimeError != nil {
				return nil, urlTimeError
			}
			url = url + urlTime
		}

		if params["interval"] != "" {
			timeString, timeStringError := util.GetString(params["interval"])
			if timeStringError != nil {
				return nil, fmt.Errorf("Start time %v. Error: %v", params["start_time"], timeStringError)
			}
			dateFormat, dateFormatErr := formatDate(timeString)
			if dateFormatErr != nil {
				return nil, dateFormatErr
			}
			params["interval"] = dateFormat
			var templ string
			if params["start_time"] == "" && params["end_time"] == "" {
				templ = templateFromTime
			} else if params["start_time"] != "" && params["end_time"] == "" {
				templ = templateUntilTime
			} else {
				return nil, fmt.Errorf("Not supported")
			}
			urlTime, urlTimeError := GetTemplateParsedString(params, templ)
			if urlTimeError != nil {
				return nil, urlTimeError
			}
			url = url + urlTime
		}

		results, getError := util.HTTPGet(url)
		if getError != nil {
			return nil, getError
		} else {
			if err := json.Unmarshal([]byte(results), &data); err != nil {
				return nil, err
			}
			return data, nil
		}
	}
}

func PushToDb(metrics interface{}) error {
	Graphite, err := graphite.NewGraphite("dhcp43-112.lab.eng.blr.redhat.com", 2003)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	m, ok := metrics.([]graphite.Metric)
	if ok {
		Graphite.SendMetrics(m)
		return nil
	}
	return fmt.Errorf("Invalid metrics %v", metrics)
}
