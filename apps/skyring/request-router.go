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
	"github.com/golang/glog"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"regexp"
)

/*
This function has the logic to find out the specific provider the request to be
routed using the route information. Route would contain specific technology name
*/
func (a *App) getProviderFromRoute(routeCfg conf.Route) *Provider {
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
				return &provider
			}
		}
	}
	return nil
}

func (a *App) getProviderFromClusterType(cluster_type string) *Provider {
	provider := a.providers[cluster_type]
	return &provider
}

func (a *App) getProviderFromClusterId(cluster_id uuid.UUID) *Provider {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.StorageCluster
	if err := collection.Find(bson.M{"clusterid": cluster_id}).One(&cluster); err != nil {
		glog.Errorf("Error getting the cluster details: %v", err)
		return nil
	}
	provider := a.providers[cluster.ClusterType]
	return &provider
}
