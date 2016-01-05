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
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"gopkg.in/mgo.v2/bson"
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
	"salt/node/appeared":                                        node_appeared_handler,
	"salt/node/lost":                                            node_lost_handler,
}

func persist_event(event models.Event) error {
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

func update_node_status(nodeStatus string, event models.Event) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"nodeid": event.NodeId}, bson.M{"$set": bson.M{"status": nodeStatus}}); err != nil {
		logger.Get().Error("Error updating the node status: %s", err)
		return err
	}
	return nil
}

func node_appeared_handler(event models.Event) error {
	if err := update_node_status(models.STATUS_UP, event); err != nil {
		return err
	}
	return nil
}

func node_lost_handler(event models.Event) error {
	if err := update_node_status(models.STATUS_DOWN, event); err != nil {
		return err
	}
	return nil
}
