/*Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package skyring

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"regexp"
	"strings"
)

/*
This function has the logic to find out the specific provider the request to be
routed using the route information. Route would contain specific technology name
*/
func (a *App) getProviderFromRoute(ctxt string, routeCfg conf.Route) *Provider {
	//Look at the URL to see if there is a match
	for _, provider := range a.providers {
		logger.Get().Debug("%s-provider:", ctxt, provider)
		//check for the URLs start with /api/v*/{provider-name}
		regex := "\\bapi/v\\d/" + provider.Name + "/"
		logger.Get().Debug("%s-regex:", ctxt, regex)
		if r, err := regexp.Compile(regex); err != nil {
			logger.Get().Error("%s-Error compiling Regex %s", ctxt, err)
			return nil
		} else {
			logger.Get().Debug("%s-Pattern:", ctxt, routeCfg.Pattern)
			if r.MatchString(routeCfg.Pattern) == true {
				return &provider
			}
		}
	}
	return nil
}

func (a *App) getProviderFromClusterType(cluster_type string) *Provider {
	if provider, ok := a.providers[cluster_type]; ok {
		return &provider
	} else {
		return nil
	}
}

func (a *App) GetProviderFromClusterId(ctxt string, cluster_id uuid.UUID) *Provider {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{"clusterid": cluster_id}).One(&cluster); err != nil {
		logger.Get().Error("%s-Error getting details for cluster: %v. error: %v", ctxt, cluster_id, err)
		return nil
	}
	if provider, ok := a.providers[cluster.Type]; ok {
		return &provider
	} else {
		return nil
	}
}

func (a *App) RouteProviderEvents(ctxt string, event models.AppEvent) error {
	provider := a.GetProviderFromClusterId(ctxt, event.ClusterId)
	if provider == nil {
		logger.Get().Error("%s-Error getting provider for cluster: %v", ctxt, event.ClusterId)
		return errors.New(fmt.Sprintf("Error getting provider for cluster: %v", event.ClusterId))
	}
	body, err := json.Marshal(event)
	if err != nil {
		logger.Get().Error("%s-Marshalling of event failed: %s", ctxt, err)
		return err
	}
	var result models.RpcResponse
	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, "ProcessEvent"),
		models.RpcRequest{RpcRequestVars: map[string]string{}, RpcRequestData: body, RpcRequestContext: ctxt},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Process evnet by Provider: %s failed. Reason :%s", ctxt, provider.Name, err)
		return err
	}
	return nil
}

func (a *App) FetchMonitoringDetailsFromProviders(ctxt string) (retVal map[string]map[string]interface{}, err error) {
	var err_str string
	if a.providers == nil || len(a.providers) == 0 {
		logger.Get().Error("%s - No providers registered", ctxt)
		return nil, fmt.Errorf("No providers registered")
	}
	retVal = make(map[string]map[string]interface{})
	for _, provider := range a.providers {
		var result models.RpcResponse
		err = provider.Client.Call(
			fmt.Sprintf("%s.%s", provider.Name, "GetSummary"),
			models.RpcRequest{RpcRequestVars: nil, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
			&result)
		if err != nil {
			err_str = fmt.Sprintf("%s %v\n", err_str, err)
			continue
		}
		if result.Status.StatusCode == http.StatusOK || result.Status.StatusCode == http.StatusPartialContent {
			providerResult := make(map[string]interface{})
			unmarshalError := json.Unmarshal(result.Data.Result, &providerResult)
			if unmarshalError != nil {
				err_str = fmt.Sprintf("%s %v\n", err_str, unmarshalError.Error())
				logger.Get().Error("%s - Error unmarshalling the monitoring data from provider %v.Error %v", ctxt, provider.Name, unmarshalError.Error())
				continue
			}
			retVal[provider.Name] = providerResult
		}
	}
	if err_str != "" {
		//Remove the trailing space or new line
		err_str = strings.TrimSpace(err_str)
		err = fmt.Errorf("%v", err_str)
	}
	return retVal, err
}

func (a *App) RouteProviderBasedMonitoring(ctxt string, cluster_id uuid.UUID) {
	provider := a.GetProviderFromClusterId(ctxt, cluster_id)
	if provider == nil {
		logger.Get().Warning("%s-Faield to get provider for cluster: %v", ctxt, cluster_id)
		return
	}
	var result models.RpcResponse

	vars := make(map[string]string)
	vars["cluster-id"] = cluster_id.String()

	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, "MonitorCluster"),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)

	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Monitoring by Provider: %s failed. Reason :%s", ctxt, provider.Name, err)
		return
	}

	return
}

func (a *App) FetchClusterDetailsFromProvider(ctxt string, clusterId uuid.UUID) (retVal map[string]map[string]interface{}, err error) {
	retVal = make(map[string]map[string]interface{})
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = clusterId.String()
	provider := a.GetProviderFromClusterId(clusterId)
	if provider == nil {
		logger.Get().Error("%s-Faield to get provider for cluster: %v", ctxt, cluster_id)
		return nil, fmt.Errorf("Faield to get provider for cluster: %v", cluster_id)
	}
	err = provider.Client.Call(fmt.Sprintf("%s.%s", provider.Name, "GetClusterSummary"), models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt}, &result)
	if err != nil {
		return nil, fmt.Errorf("%s - Call to provider %v failed.Err: %v\n", ctxt, provider.Name, err)
	}
	if result.Status.StatusCode == http.StatusOK || result.Status.StatusCode == http.StatusPartialContent {
		providerResult := make(map[string]interface{})
		unmarshalError := json.Unmarshal(result.Data.Result, &providerResult)
		if unmarshalError != nil {
			logger.Get().Error("%s - Error unmarshalling the monitoring data from provider %v.Error %v", ctxt, provider.Name, unmarshalError.Error())
			return nil, fmt.Errorf("%s - Error unmarshalling the monitoring data from provider %v.Error %v", ctxt, provider.Name, unmarshalError.Error())
		}
		retVal[provider.Name] = providerResult
	}
	return retVal, err
}
