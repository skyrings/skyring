package graphitemanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/marpaia/graphite-golang"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/utils"
	"io"
	"regexp"
	"strconv"
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

var (
	templateUrl             = "http://{{.hostname}}:{{.port}}/render?target={{.target}}&format=json"
	targetLatest            = "cactiStyle({{.targetGeneral}})"
	targetGeneral           = "{{.collectionname}}.{{.nodename}}.{{.resourcename}}"
	targetWildCard          = "*.*"
	templateFromTime        = "&from={{.start_time}}"
	templateUntilTime       = "&until={{.end_time}}"
	timeFormat              = "15:04_20060102"
	standardTimeFormat      = "2006-01-02T15:04:05.000Z"
	latest                  = "latest"
	fullQualifiedMetricName bool
)

var SupportedInputTimeFormats = []string{
	"2006-01-02T15:04:05.000Z",
	"2006-01-02",
	"20060102",
}

type GraphiteMetric struct {
	Target string `json:"target"`
	Stats  stats  `json:"datapoints"`
}

type stat []interface{}
type stats []stat

func (slice stats) Len() int {
	return len(slice)
}

func (slice stats) Less(i, j int) bool {
	s1, _ := slice[i][1].(int32)
	s2, _ := slice[j][1].(int32)
	return s1 < s2
}

func (slice stats) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

type GraphiteMetrics []GraphiteMetric

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
	target, targetErr := GetTemplateParsedString(map[string]interface{}{
		"collectionname": conf.SystemConfig.TimeSeriesDBConfig.CollectionName,
		"nodename":       nodename,
		"resourcename":   params["resource"],
	}, targetGeneral)
	if targetErr != nil {
		return nil, targetErr
	}

	if !fullQualifiedMetricName {
		target = target + targetWildCard
	}

	if params["interval"] == latest {
		target, targetErr = GetTemplateParsedString(map[string]interface{}{"targetGeneral": target}, targetLatest)
		if targetErr != nil {
			return nil, targetErr
		}
	}

	return map[string]interface{}{
		"hostname": conf.SystemConfig.TimeSeriesDBConfig.Hostname,
		"port":     conf.SystemConfig.TimeSeriesDBConfig.Port,
		"target":   target,
	}, nil
}

func Matches(key string, keys []string) bool {
	for _, permittedKey := range keys {
		if strings.Index(key, permittedKey) == 0 {
			if key != permittedKey {
				fullQualifiedMetricName = true
			}
			return true
		}
	}
	return false
}

func (tsdbm GraphiteManager) QueryDB(params map[string]interface{}) (interface{}, error) {
	var resource string
	if str, ok := params["resource"].(string); ok {
		resource = str
	}
	if !Matches(resource, monitoring.GeneralResources) {
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

		if params["interval"] != "" && params["interval"] != latest {
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
			urlTime, urlTimeError := GetTemplateParsedString(map[string]interface{}{"start_time": params["interval"]}, templ)
			if urlTimeError != nil {
				return nil, urlTimeError
			}
			url = url + urlTime
		}

		results, getError := util.HTTPGet(url)
		if getError != nil {
			return nil, getError
		} else {
			var metrics []GraphiteMetric
			if mErr := json.Unmarshal(results, &metrics); mErr != nil {
				return nil, fmt.Errorf("Error unmarshalling the metrics %v.Error:%v", metrics, mErr)
			}
			if params["interval"] == latest {
				/*
					The output of cactistyle graphite function(It gives summarised output including max value, min value and current non-nil value) is as under:
					[{"target": "<metric_name>     Current:<current_val>      Max:<max>      Min:<min>      ", "datapoints": [[val1, timestamp1],...]}
					....
					]
				*/

				for metricIndex, metric := range metrics {
					target := metric.Target
					if !strings.Contains(target, "Current:") {
						return nil, fmt.Errorf("Current non-nil value is not available")
					}
					targetContents := strings.Fields(target)

					index := -1
					for contentIndex, content := range targetContents {
						if strings.Contains(content, "Current:") {
							index = contentIndex
							break
						}
					}

					if index == -1 {
						return nil, fmt.Errorf("Current non-nil value is not available")
					}

					statValues := strings.Split(targetContents[index], ":")
					if len(statValues) != 2 {
						return nil, fmt.Errorf("Current non-nil value is not available")
					}

					var statVar []interface{}
					statVar = append(statVar, statValues[1])
					/*
						Effectively the current value returned by graphite is actually the last non-nil value in graphite for the metric
						Since graphite doesn't return the time stamp of current value it is felt safe to insert a 0 for the field,
						instead of meddling into looking at when actually the currentvalue appears in the possibly humongously huge set of data values
					*/
					statVar = append(statVar, 0)

					metrics[metricIndex].Target = targetContents[0]
					metrics[metricIndex].Stats = stats([]stat{statVar})
				}
			}
			return metrics, nil
		}
	}
}

//This method takes map[string]map[string]string ==> map[metric/table name]map[timestamp]value
func (tsdbm GraphiteManager) PushToDb(metrics map[string]map[string]string, hostName string, port int) error {
	data := make([]graphite.Metric, 1)
	for tableName, valueMap := range metrics {
		for timestamp, value := range valueMap {
			timeInt, err := strconv.ParseInt(timestamp, 10, 64)
			if err != nil {
				return fmt.Errorf("Failed to parse timestamp %v of metric tableName %v.Error: %v", timestamp, tableName, err.Error())
			}
			data = append(data, graphite.Metric{Name: tableName, Value: value, Timestamp: timeInt})
		}
	}
	Graphite, err := graphite.NewGraphite(hostName, port)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	Graphite.SendMetrics(data)
	return nil
}
