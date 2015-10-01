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
	"github.com/golang/glog"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"gopkg.in/mgo.v2/bson"
	"regexp"
)

/*
This function has the logic to find out the specific provider the request to be
routed. All the generic cases are covered here. But if any of the new endpoints
has any specific logic, that needs to be added here.
case 1: Technology specific APIs, has the name in the URL and if there is match
        route it to the specific provider
case 2: Looking at the cluster type in the body of the request
case 3: Looking at the cluster where operation is done
*/
func (a *App) getProvider(body []byte, routeCfg conf.Route) *Provider {
	var result *Provider = nil
	//Look at the URL to see if there is a match
	for _, provider := range a.providers {
		glog.V(3).Infof("provider:", provider)
		//check for the URLs start with /api/v*/{provider-name}
		regex := "\\bapi/v\\d/" + provider.Name + "/"
		glog.V(3).Infof("regex:", regex)
		if r, err := regexp.Compile(regex); err != nil {
			glog.Errorf("Error compiling Regex %s", err)
			return nil
		} else {
			glog.V(3).Infof("Pattern:", routeCfg.Pattern)
			if r.MatchString(routeCfg.Pattern) == true {
				result = &provider
				return result
			}
		}
	}
	var request map[string]string
	if err := json.Unmarshal(body, &request); err != nil {
		glog.Errorf("Error un-marshaling request %s", err)
		return nil
	}
	// case 2: Looking at the cluster type in the body of the request
	if result == nil {
		provider := a.providers[request["cluster_type"]]
		return &provider
	}

	// Looking at the cluster where operation is done - cluster-id
	if result == nil {
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()

		collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
		var cluster models.StorageCluster
		if err := collection.Find(bson.M{"clusterid": request["cluster_id"]}).One(&cluster); err != nil {
			glog.Errorf("Error getting the cluster details: %v", err)
			return nil
		}
		provider := a.providers[cluster.ClusterType]
		return &provider
	}
	return nil
}
