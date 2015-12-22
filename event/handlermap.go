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
package event

import (
	"encoding/json"
	"fmt"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"gopkg.in/mgo.v2/bson"
	"net/http"
)

var handlermap = map[string]interface{}{
	"skyring/dbus/node/*/generic/storage/block/added":           block_add_handler,
	"skyring/dbus/node/*/generic/storage/block/removed":         block_remove_handler,
	"skyring/dbus/node/*/generic/storage/block/changed":         block_change_handler,
	"skyring/dbus/node/*/generic/storage/mount/changed":         mount_change_handler,
	"skyring/dbus/node/*/generic/storage/drive/added":           drive_add_handler,
	"skyring/dbus/node/*/generic/storage/drive/removed":         drive_remove_handler,
	"skyring/dbus/node/*/generic/storage/drive/possibleFailure": drive_remove_handler,
	"skyring/dbus/node/*/generic/service/collectd":              collectd_status_handler,
}

func Persist_event(event models.Event) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_NODE_EVENTS)
	if err := coll.Insert(event); err != nil {
		logger.Get().Error("Error adding the node event: %v", err)
		return err
	}
	return nil
}

// ALL HANDLERS ARE JUST WRITING THE EVENTS TO DB. OTHER HANDLING AND CORRELATION
// IS TO BE DONE

func block_add_handler(event models.Event) error {
	return nil
}

func block_remove_handler(event models.Event) error {
	return nil
}

func block_change_handler(event models.Event) error {
	return nil
}

func mount_change_handler(event models.Event) error {
	return nil
}

func drive_add_handler(event models.Event) error {
	return nil
}

func drive_remove_handler(event models.Event) error {
	return nil
}

func collectd_status_handler(event models.Event) error {
	return nil
}

func provider_events(event models.Event) error {
	var cluster models.Cluster
	var provider skyring.Provider
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := coll.Find(bson.M{"clusterid": event.ClusterId}).One(&cluster); err != nil {
		logger.Get().Error("Cluster information read from DB failed: %s", err)
		return err
	}

	provider = skyring.GetProviderFromName(cluster.Type)
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
