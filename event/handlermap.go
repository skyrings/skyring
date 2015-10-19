package event

import (
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/conf"
)

var handlermap = map[string]interface{} {
	"dbus/node/*/generic/storage/block/added" : block_add_handler,
	"dbus/node/*/generic/storage/block/removed" : block_remove_handler,
	"dbus/node/*/generic/storage/block/changed" : block_change_handler,
	"dbus/node/*/generic/storage/mount/changed" : mount_change_handler,
	"dbus/node/*/generic/storage/drive/added" : drive_add_handler,
	"dbus/node/*/generic/storage/drive/removed" : drive_remove_handler,
	"dbus/node/*/generic/storage/drive/possibleFailure" : drive_remove_handler,
	"dbus/node/*/glusterfs/service/glusterd" : glusterd_status_handler,
}


func persist_event(event  models.NodeEventDoc) error {
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

func block_add_handler(event  models.NodeEventDoc) error {
	if err := persist_event(event); err != nil {
		return err
	} else {
		return nil
	}
}

func glusterd_status_handler(event  models.NodeEventDoc) error {
	if err := persist_event(event); err  != nil {
		return err
	} else {
		return nil
	}
}

func block_remove_handler(event  models.NodeEventDoc) error {
	if err := persist_event(event); err  != nil {
		return err
	} else {
		return nil
	}
}

func block_change_handler(event  models.NodeEventDoc) error {
	if err := persist_event(event); err  != nil {
		return err
	} else {
		return nil
	}
}

func mount_change_handler(event  models.NodeEventDoc) error {
	if err := persist_event(event); err  != nil {
		return err
	} else {
		return nil
	}
}

func drive_add_handler(event  models.NodeEventDoc) error {
	if err := persist_event(event); err  != nil {
		return err
	} else {
		return nil
	}
}

func drive_remove_handler(event  models.NodeEventDoc) error {
	if err := persist_event(event); err  != nil {
		return err
	} else {
		return nil
	}
}
