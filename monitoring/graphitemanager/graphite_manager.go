package graphitemanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/utils"
	"io"
	"strings"
	"text/template"
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

var GeneralResources = []string{
	"df",
	"memory",
	"cpu",
}

var (
	templateUrl       = "http://{{.hostname}}:{{.port}}/render?target={{.collectionname}}.{{.nodename}}.{{.resourcename}}*.*&format=json"
	templateFromTime  = "&from={{.fromTime}}"
	templateUntilTime = "&until={{.untilTime}}"
)

func GetTemplateParsedString(urlParams map[string]interface{}, templateString string) (string, error) {
	buf := new(bytes.Buffer)
	parsedTemplate, err := template.New("graphite_url_parser").Parse(templateString)
	if err != nil {
		return "", err
	}
	parsedTemplate.Execute(buf, urlParams)
	return buf.String(), nil
}

func NewGraphiteManager(config io.Reader) (*GraphiteManager, error) {
	return &GraphiteManager{}, nil
}

func getUrlBaseTemplateParams(params map[string]interface{}) map[string]interface{} {
	var nodename string
	if str, ok := params["nodename"].(string); ok {
		nodename = str
	}
	nodename = strings.Replace(nodename, ".", "_", -1)
	return map[string]interface{}{
		"hostname":       conf.SystemConfig.TimeSeriesDBConfig.Hostname,
		"port":           conf.SystemConfig.TimeSeriesDBConfig.Port,
		"collectionname": conf.SystemConfig.TimeSeriesDBConfig.CollectionName,
		"nodename":       nodename,
		"resourcename":   params["resource"],
	}
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
		url, urlError := GetTemplateParsedString(getUrlBaseTemplateParams(params), templateUrl)
		if urlError != nil {
			return nil, urlError
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
