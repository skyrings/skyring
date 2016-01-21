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
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"regexp"
)

/*
This function has the logic to find out the specific provider the request to be
routed using the route information. Route would contain specific technology name
*/
func (a *App) getProviderFromRoute(routeCfg conf.Route) *Provider {
	//Look at the URL to see if there is a match
	for _, provider := range a.providers {
		logger.Get().Debug("provider:", provider)
		//check for the URLs start with /api/v*/{provider-name}
		regex := "\\bapi/v\\d/" + provider.Name + "/"
		logger.Get().Debug("regex:", regex)
		if r, err := regexp.Compile(regex); err != nil {
			logger.Get().Error("Error compiling Regex %s", err)
			return nil
		} else {
			logger.Get().Debug("Pattern:", routeCfg.Pattern)
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

func (a *App) getProviderFromClusterId(cluster_id uuid.UUID) *Provider {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{"clusterid": cluster_id}).One(&cluster); err != nil {
		logger.Get().Error("Error getting the cluster details: %v", err)
		return nil
	}
	if provider, ok := a.providers[cluster.Type]; ok {
		return &provider
	} else {
		return nil
	}
}

func (a *App) RouteProviderEvents(event models.Event) error {
	provider := a.getProviderFromClusterId(event.ClusterId)
	if provider == nil {
		logger.Get().Error("Error getting provider for cluster: %v", event.ClusterId)
		return errors.New(fmt.Sprintf("Error getting provider for cluster: %v", event.ClusterId))
	}
	body, err := json.Marshal(event)
	if err != nil {
		logger.Get().Error("Marshalling of event failed: %s", err)
		return err
	}
	var result models.RpcResponse
	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, "ProcessEvent"),
		models.RpcRequest{RpcRequestVars: map[string]string{}, RpcRequestData: body},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("Process evnet by Provider: %s failed. Reason :%s", provider.Name, err)
		return err
	}
	return nil
}
