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
	"github.com/gorilla/context"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/lock"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/task"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring-common/utils"
	"github.com/skyrings/skyring/nodemanager/saltnodemanager"
	"github.com/skyrings/skyring/skyringutils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"regexp"
	"time"
)

type APIError struct {
	Error string
}

func lockNode(ctxt string, nodeId uuid.UUID, hostname string, operation string) (*lock.AppLock, error) {
	//lock the node
	locks := make(map[uuid.UUID]string)
	if nodeId.IsZero() {
		//Generate temporary UUID from hostname for the node
		//for locking as the UUID is not available at this point
		id, err := uuid.Parse(util.Md5FromString(hostname))
		if err != nil {
			return nil, fmt.Errorf("Unable to create the UUID for locking for host: %s. error: %v", hostname, err)
		}
		nodeId = *id
	}
	locks[nodeId] = fmt.Sprintf("%s : %s", operation, hostname)
	appLock := lock.NewAppLock(locks)
	if err := GetApp().GetLockManager().AcquireLock(ctxt, *appLock); err != nil {
		return nil, err
	}
	return appLock, nil
}

func LockNodes(ctxt string, nodes models.Nodes, operation string) (*lock.AppLock, error) {
	locks := make(map[uuid.UUID]string)
	for _, node := range nodes {
		locks[node.NodeId] = fmt.Sprintf("%s : %s", operation, node.Hostname)
	}
	appLock := lock.NewAppLock(locks)
	if err := GetApp().GetLockManager().AcquireLock(ctxt, *appLock); err != nil {
		return nil, err
	}
	return appLock, nil
}

func getClusterNodesById(cluster_id *uuid.UUID) (models.Nodes, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
		return nil, err
	}
	return nodes, nil

}

func getClusterNodesFromRequest(clusterNodes []models.ClusterNode) (models.Nodes, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	var nodes models.Nodes
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	for _, clusterNode := range clusterNodes {
		uuid, err := uuid.Parse(clusterNode.NodeId)
		if err != nil {
			return nodes, err
		}
		var node models.Node
		if err := coll.Find(bson.M{"nodeid": *uuid}).One(&node); err != nil {
			return nodes, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil

}

func HandleHttpError(rw http.ResponseWriter, err error) {
	bytes, _ := json.Marshal(APIError{Error: err.Error()})
	rw.WriteHeader(http.StatusInternalServerError)
	rw.Write(bytes)
}

func HttpResponse(w http.ResponseWriter, status_code int, msg string, args ...string) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.WriteHeader(status_code)
	var ctxt string
	if len(args) > 0 {
		ctxt = args[0]
	}
	if err := json.NewEncoder(w).Encode(msg); err != nil {
		logger.Get().Error("%s-Error: %v", ctxt, err)
	}
	return
}

func GetContext(r *http.Request) (string, error) {

	val, ok := context.GetOk(r, LoggingCtxt)
	if !ok {
		return "", errors.New("Error Geting the Context")
	}
	token, ok := val.(string)
	if !ok {
		return "", errors.New("Error Geting the Context")
	}
	return token, nil
}

func Paginate(pageNo int, pageSize int, apiLimit int) (startIndex int, endIndex int) {
	if pageNo < 1 {
		pageNo = 1
	}

	if pageSize < 1 || pageSize > apiLimit {
		pageSize = apiLimit
	}
	startIndex = (pageNo - 1) * pageSize
	endIndex = pageSize + startIndex - 1
	return startIndex, endIndex
}

func GetCluster(cluster_id *uuid.UUID) (cluster models.Cluster, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).One(&cluster); err != nil {
		return cluster, err
	}
	return cluster, nil
}

func getNodesInCluster(cluster_id *uuid.UUID) (cluster_node_names []string, err error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := collection.Find(bson.M{"clusterid": *cluster_id}).All(&nodes); err != nil {
		return nil, err
	}
	for _, node := range nodes {
		cluster_node_names = append(cluster_node_names, node.Hostname)
	}
	return cluster_node_names, nil
}

func cluster_exists(key string, value string) (*models.Cluster, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{key: value}).One(&cluster); err != nil {
		return nil, err
	} else {
		return &cluster, nil
	}
}

func ClusterUnmanaged(cluster_id uuid.UUID) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var cluster models.Cluster
	if err := collection.Find(bson.M{"clusterid": cluster_id}).One(&cluster); err != nil {
		return false, err
	}
	if cluster.State == models.CLUSTER_STATE_UNMANAGED {
		return true, nil
	} else {
		return false, nil
	}
}

func GetSLU(cluster_id *uuid.UUID, slu_id uuid.UUID) (slu models.StorageLogicalUnit, err error) {
	if cluster_id == nil {
		return slu, fmt.Errorf("Cluster Id not available for slu with id %v", slu_id)
	}
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
	if err := coll.Find(bson.M{"clusterid": *cluster_id, "sluid": slu_id}).One(&slu); err != nil {
		return slu, fmt.Errorf("Error getting the slu: %v for cluster: %v. error: %v", slu_id, *cluster_id, err)
	}
	if slu.Name == "" {
		return slu, fmt.Errorf("Slu: %v not found for cluster: %v", slu_id, *cluster_id)
	}
	return slu, nil
}

func GetClusters() (models.Clusters, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()

	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.Clusters
	err := collection.Find(nil).All(&clusters)
	return clusters, err
}

func syncNodeStatus(ctxt string, node models.Node) error {
	skyringutils.Update_node_state_byId(ctxt, node.NodeId, models.NODE_STATE_ACTIVE)
	//get the latest status
	ok, err := GetCoreNodeManager().IsNodeUp(node.Hostname, ctxt)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("Error getting status of node: %s. error: %v", node.Hostname, err))
		return nil
	}
	if ok {
		skyringutils.Update_node_status_byId(ctxt, node.NodeId, models.NODE_STATUS_OK)
	} else {
		skyringutils.Update_node_status_byId(ctxt, node.NodeId, models.NODE_STATUS_ERROR)
	}

	//TODO update Alaem status and count

	return nil
}

func syncClusterStatus(ctxt string, cluster_id *uuid.UUID) error {
	provider := GetApp().GetProviderFromClusterId(ctxt, *cluster_id)
	if provider == nil {
		logger.Get().Error("Error getting provider for the cluster: %s", *cluster_id)
		return errors.New("Error getting the provider")
	}
	cluster, err := GetCluster(cluster_id)
	if err != nil {
		logger.Get().Error("Error getting cluster details for the cluster: %s", *cluster_id)
		return err
	}
	success, err := sync_cluster_status(ctxt, cluster, provider)
	if !success || err != nil {
		logger.Get().Error("Error updating cluster status for the cluster: %s", cluster.Name)
	}
	return nil
}

func Initialize(node string, ctxt string) error {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)

	var storage_node models.Node

	_ = coll.Find(bson.M{"hostname": node}).One(&storage_node)
	if storage_node.State != models.NODE_STATE_INITIALIZING {
		logger.Get().Warning(fmt.Sprintf("%s-Node with name: %s not in intializing state to update other details", ctxt, node))
		return nil
	}
	asyncTask := func(t *task.Task) {
		for {
			select {
			case <-t.StopCh:
				return
			default:
				var nodeId uuid.UUID
				appLock, err := lockNode(ctxt, nodeId, node, "Accepted_Nodes")
				if err != nil {
					util.FailTask("Failed to acquire lock", fmt.Errorf("%s-%v", ctxt, err), t)
					return
				}
				defer GetApp().GetLockManager().ReleaseLock(ctxt, *appLock)
				t.UpdateStatus("started the task for InitializeNode: %s", t.ID)
				// Process the request
				if err := initializeStorageNode(storage_node.Hostname, t, ctxt); err != nil {
					t.UpdateStatus("Failed")
					t.Done(models.TASK_STATUS_FAILURE)
				} else {
					t.UpdateStatus("Success")
					t.Done(models.TASK_STATUS_SUCCESS)
				}
				return
			}
		}
	}
	if taskId, err := GetApp().GetTaskManager().Run(fmt.Sprintf("Initialize Node: %s", storage_node.Hostname), asyncTask, 600*time.Second, nil, nil, nil); err != nil {
		logger.Get().Error("%s-Unable to create the task for Initialize Node: %s. error: %v", ctxt, storage_node.Hostname, err)
		return err
	} else {
		logger.Get().Debug("%s-Task created for initialize node. Task id: %s", ctxt, taskId.String())
	}
	return nil
}

func initializeStorageNode(node string, t *task.Task, ctxt string) error {
	sProfiles, err := GetDbProvider().StorageProfileInterface().StorageProfiles(ctxt, nil, models.QueryOps{})
	if err != nil {
		logger.Get().Error("%s-Unable to get the storage profiles. May not be able to apply storage profiles for node: %v err:%v", ctxt, node, err)
	}
	if storage_node, ok := saltnodemanager.GetStorageNodeInstance(node, sProfiles, ctxt); ok {
		if nodeErrorMap, configureError := GetCoreNodeManager().SetUpMonitoring(
			node,
			curr_hostname,
			ctxt); configureError != nil && len(nodeErrorMap) != 0 {
			if len(nodeErrorMap) != 0 {
				logger.Get().Error("%s-Unable to setup collectd on %s because of %v", ctxt, node, nodeErrorMap)
				t.UpdateStatus("Unable to setup collectd on %s because of %v", node, nodeErrorMap)
				skyringutils.UpdateNodeState(ctxt, node, models.NODE_STATE_FAILED)
				skyringutils.UpdateNodeStatus(ctxt, node, models.NODE_STATUS_UNKNOWN)
				return err
			} else {
				logger.Get().Error("%s-Config Error during monitoring setup for node:%s Error:%v", ctxt, node, configureError)
				t.UpdateStatus("Config Error during monitoring setup for node:%s Error:%v", node, configureError)
				skyringutils.UpdateNodeState(ctxt, node, models.NODE_STATE_FAILED)
				skyringutils.UpdateNodeStatus(ctxt, node, models.NODE_STATUS_UNKNOWN)
				return err
			}
		}
		if ok, err := GetCoreNodeManager().SyncModules(node, ctxt); !ok || err != nil {
			logger.Get().Error("%s-Failed to sync modules on the node: %s. error: %v", ctxt, node, err)
			t.UpdateStatus("Failed to sync modules")
			skyringutils.UpdateNodeState(ctxt, node, models.NODE_STATE_FAILED)
			skyringutils.UpdateNodeStatus(ctxt, node, models.NODE_STATUS_UNKNOWN)
			return err
		}
		if err := saltnodemanager.SetupSkynetService(node, ctxt); err != nil {
			logger.Get().Error("%s-Failed to setup skynet service on the node: %s. error: %v", ctxt, node, err)
			t.UpdateStatus("Failed to setup skynet service")
			skyringutils.UpdateNodeState(ctxt, node, models.NODE_STATE_FAILED)
			skyringutils.UpdateNodeStatus(ctxt, node, models.NODE_STATUS_UNKNOWN)
			return err
		}
		if err := updateStorageNodeToDB(*storage_node, ctxt); err != nil {
			logger.Get().Error("%s-Unable to add details of node: %s to DB. error: %v", ctxt, node, err)
			t.UpdateStatus("Unable to add details of node: %s to DB. error: %v", node, err)
			skyringutils.UpdateNodeState(ctxt, node, models.NODE_STATE_FAILED)
			skyringutils.UpdateNodeStatus(ctxt, node, models.NODE_STATUS_UNKNOWN)
			return err
		}
		return nil
	} else {
		logger.Get().Critical("%s-Error getting the details for node: %s", ctxt, node)
		t.UpdateStatus("Error getting the details for node: %s", node)
		skyringutils.UpdateNodeState(ctxt, node, models.NODE_STATE_FAILED)
		skyringutils.UpdateNodeStatus(ctxt, node, models.NODE_STATUS_UNKNOWN)
		return fmt.Errorf("Error getting the details for node: %s", node)
	}
}

func updateStorageNodeToDB(storage_node models.Node, ctxt string) error {
	// Add the node details to the DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var node models.Node
	err := coll.Find(bson.M{"nodeid": storage_node.NodeId}).One(&node)
	if err == mgo.ErrNotFound {
		storage_node.State = models.NODE_STATE_ACTIVE
		if err := coll.Update(bson.M{"hostname": storage_node.Hostname}, storage_node); err != nil {
			logger.Get().Critical("%s-Error Updating the node: %s. error: %v", ctxt, storage_node.Hostname, err)
			return err
		}
		return nil
	} else {
		logger.Get().Critical(fmt.Sprintf("%s-Node with id: %v already exists", ctxt, storage_node.NodeId))
		return errors.New(fmt.Sprintf("Node with id: %v already exists", storage_node.NodeId))
	}
}

func valid_storage_size(size string) (bool, error) {
	matched, err := regexp.Match(
		"^([0-9])*MB$|^([0-9])*mb$|^([0-9])*GB$|^([0-9])*gb$|^([0-9])*TB$|^([0-9])*tb$|^([0-9])*PB$|^([0-9])*pb$",
		[]byte(size))
	if err != nil {
		return false, errors.New(fmt.Sprintf("Error parsing the size: %s", size))
	}
	if !matched {
		return false, errors.New(fmt.Sprintf("Invalid format size: %s", size))
	}
	return true, nil
}
