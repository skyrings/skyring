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
)

var handlermap = map[string]interface{}{
	"dbus/node/*/generic/storage/block/added":           block_add_handler,
	"dbus/node/*/generic/storage/block/removed":         block_remove_handler,
	"dbus/node/*/generic/storage/block/changed":         block_change_handler,
	"dbus/node/*/generic/storage/mount/changed":         mount_change_handler,
	"dbus/node/*/generic/storage/drive/added":           drive_add_handler,
	"dbus/node/*/generic/storage/drive/removed":         drive_remove_handler,
	"dbus/node/*/generic/storage/drive/possibleFailure": drive_remove_handler,
	"dbus/node/*/glusterfs/service/glusterd":            glusterd_status_handler,
	"skyring/calamari/ceph/calamari/started":            calamari_server_start_handler,
	"skyring/calamari/ceph/server/added":                ceph_server_add_handler,
	"skyring/calamari/ceph/server/reboot":               ceph_server_reboot_handler,
	"skyring/calamari/ceph/server/package/changed":      ceph_server_package_change_handler,
	"skyring/calamari/ceph/server/lateReporting":        ceph_server_late_reporting_handler,
	"skyring/calamari/ceph/server/regainedContact":      ceph_server_contact_regained_handler,
	"skyring/calamari/ceph/cluster/lateReporting":       ceph_cluster_late_reporting_handler,
	"skyring/calamari/ceph/cluster/regainedContact":     ceph_cluster_contact_regained_handler,
	"skyring/calamari/ceph/osd/propertyChanged":         ceph_osd_property_changed_handler,
	"skyring/calamari/ceph/mon/propertyChanged":         ceph_mon_property_changed_handler,
	"skyring/calamari/ceph/cluster/health/changed":      ceph_cluster_health_changed,
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

func glusterd_status_handler(event models.Event) error {
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

func calamari_server_start_handler(event models.Event) error {
	return nil
}

func ceph_server_add_handler(event models.Event) error {
	return nil
}

func ceph_server_reboot_handler(event models.Event) error {
	return nil
}

func ceph_server_package_change_handler(event models.Event) error {
	return nil
}

func ceph_server_late_reporting_handler(event models.Event) error {
	return nil
}

func ceph_server_contact_regained_handler(event models.Event) error {
	return nil
}

func ceph_cluster_late_reporting_handler(event models.Event) error {
	return nil
}

func ceph_cluster_contact_regained_handler(event models.Event) error {
	return nil
}

func ceph_osd_property_changed_handler(event models.Event) error {
	return nil
}

func ceph_mon_property_changed_handler(event models.Event) error {
	return nil
}

func ceph_cluster_health_changed(event models.Event) error {
	return nil
}
