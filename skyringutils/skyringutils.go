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
package skyringutils

import (
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"gopkg.in/mgo.v2/bson"
)

func UpdateNodeState(ctxt string, node string, state models.NodeState) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"state": state}}); err != nil {
		logger.Get().Critical("%s-Error updating the node state for node: %s. error: %v", ctxt, node, err)
		return err
	}
	return nil
}

func UpdateNodeStatus(ctxt string, node string, status models.NodeStatus) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"status": status}}); err != nil {
		logger.Get().Critical("%s-Error updating the node state for node: %s. error: %v", ctxt, node, err)
		return err
	}
	return nil
}

func Update_node_status_byId(ctxt string, nodeId uuid.UUID, nodeStatus models.NodeStatus) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"nodeid": nodeId}, bson.M{"$set": bson.M{"status": nodeStatus}}); err != nil {
		logger.Get().Error("%s-Error updating the node status: %s", ctxt, err)
		return err
	}
	return nil
}

func GetNodeStateById(ctxt string, nodeId uuid.UUID) (models.NodeState, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"nodeid": nodeId}).One(&node); err != nil {
		logger.Get().Critical("%s-Error Getting the node : %s. error: %v", ctxt, nodeId, err)
		return 0, err
	}
	return node.State, nil
}

func Update_node_state_byId(ctxt string, nodeId uuid.UUID, state models.NodeState) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"nodeid": nodeId}, bson.M{"$set": bson.M{"state": state}}); err != nil {
		logger.Get().Error("%s-Error updating the node state: %s", ctxt, err)
		return err
	}
	return nil
}
