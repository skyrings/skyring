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
package events

import (
	"encoding/json"
	"github.com/golang/glog"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/utils"
	"gopkg.in/mgo.v2/bson"
	"net/http"
)

func GetEvents(rw http.ResponseWriter, req *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_NODE_EVENTS)
	var events []models.NodeEvent

	err := collection.Find(bson.M{}).All(&events)
	if err != nil {
		glog.Errorf("Error getting record from DB:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	bytes, err := json.Marshal(events)
	if err != nil {
		glog.Errorf("Unable to marshal the list of events:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)
}
