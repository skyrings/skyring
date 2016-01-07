package graphitemanager

import (
	"bytes"
	"fmt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/monitoring"
	"github.com/skyrings/skyring/utils"
	"io"
	"io/ioutil"
	"net/http"
	"text/template"
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
	return map[string]interface{}{
		"hostname":       conf.SystemConfig.TimeSeriesDBConfig.Hostname,
		"port":           conf.SystemConfig.TimeSeriesDBConfig.Port,
		"collectionname": conf.SystemConfig.TimeSeriesDBConfig.CollectionName,
		"nodename":       params["nodename"],
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
	if !Matches(params["resource"], GeneralResources) {
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
		return util.HTTPGet(GetTemplateParsedString(getUrlBaseTemplateParams(params))), nil
	}
}
