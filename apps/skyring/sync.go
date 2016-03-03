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
	"fmt"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"strconv"
)

var (
	syc_functions = map[string]string{
		"cluster_status":     "GetClusterStatus",
		"sync_nodes":         "SyncStorageNodes",
		"sync_storages":      "SyncStorages",
		"sync_slus":          "SyncStorageLogicalUnits",
		"sync_block_devices": "SyncBlockDevices",
	}
)

func (a *App) SyncClusterDetails(params map[string]interface{}) {
	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the Request Id for context. error: %v", err)
	}
	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())

	// Get the list of cluster
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.Clusters
	if err := coll.Find(nil).All(&clusters); err != nil {
		logger.Get().Error("%s-Error getting the clusters list. Unable to sync details. error: %v", ctxt, err)
		return
	}
	for _, cluster := range clusters {
		if cluster.State != models.CLUSTER_STATE_ACTIVE {
			logger.Get().Info("%s-Cluster %s is not in active state. Skipping sync.", ctxt, cluster.Name)
			continue
		}

		// Change the state of the cluster as syncing
		logger.Get().Debug("Setting the state of cluster: %s as syncing", cluster.Name)
		if err := coll.Update(
			bson.M{"clusterid": cluster.ClusterId},
			bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_SYNCING}}); err != nil {
			logger.Get().Error("%s-Error marking the cluster %s as syncing. error: %s", ctxt, cluster.Name, err)
			continue
		}

		// Lock the cluster
		appLock, err := LockCluster(ctxt, cluster, "SyncClusterDetails")
		if err != nil {
			logger.Get().Error("Failed to acquire lock for cluster: %s. error: %v", cluster.Name, err)
			continue
		}
		defer a.GetLockManager().ReleaseLock(ctxt, *appLock)

		provider := a.GetProviderFromClusterId(ctxt, cluster.ClusterId)
		if provider == nil {
			logger.Get().Error("%s-Error getting provider for the cluster: %s", ctxt, cluster.Name)
			continue
		}

		// Sync the cluster status
		logger.Get().Debug("Syncing status of cluster: %s", cluster.Name)
		if ok, err := sync_cluster_status(ctxt, cluster, provider); err != nil || !ok {
			logger.Get().Error("%s-Error updating status for cluster: %s", ctxt, cluster.Name)
		}

		// Sync the cluster nodes
		logger.Get().Debug("Syncing nodes of cluster: %s", cluster.Name)
		if ok, err := sync_cluster_nodes(ctxt, cluster, provider); err != nil || !ok {
			logger.Get().Error("%s-Error syncing storage nodes for cluster: %s", ctxt, cluster.Name)
		}

		// Sync the cluster status
		logger.Get().Debug("Syncing SLUs of cluster: %s", cluster.Name)
		if ok, err := syncSlus(ctxt, cluster, provider); err != nil || !ok {
			logger.Get().Error("%s-Error syncing slus: %s", ctxt, cluster.Name)
		}

		// Sync the storage entities of the cluster
		logger.Get().Debug("Syncing storages of cluster: %s", cluster.Name)
		if ok, err := sync_cluster_storage_entities(ctxt, cluster, provider); err != nil || !ok {
			logger.Get().Error("%s-Error syncing storage entities for cluster: %s. error: %v", ctxt, cluster.Name, err)
		}
		// Sync block devices
		logger.Get().Debug("Syncing block devices of cluster: %s", cluster.Name)
		if ok, err := sync_block_devices(ctxt, cluster, provider); err != nil || !ok {
			logger.Get().Error("%s-Error syncing block devices for cluster: %s. error: %v", ctxt, cluster.Name, err)
		}

		logger.Get().Debug("Setting the cluster: %s back as active", cluster.Name)
		if err := coll.Update(
			bson.M{"clusterid": cluster.ClusterId},
			bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
			logger.Get().Error("%s-Error setting the back cluster state. error: %v", ctxt, err)
		}
	}
}

func sync_cluster_status(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()
	err = provider.Client.Call(
		fmt.Sprintf("%s.%s", provider.Name, syc_functions["cluster_status"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error getting status for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}
	clusterStatus, err := strconv.Atoi(string(result.Data.Result))
	logger.Get().Debug("Cluster Status: %d", clusterStatus)
	if err != nil {
		logger.Get().Error("%s-Error getting status for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	// Set the cluster status
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	logger.Get().Info("%s-Updating the status of the cluster: %s to %d", ctxt, cluster.Name, clusterStatus)
	if err := coll.Update(bson.M{"clusterid": cluster.ClusterId}, bson.M{"$set": bson.M{"status": clusterStatus}}); err != nil {
		logger.Get().Error("%s-Error updating status for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}
	logger.Get().Debug("Done")

	return true, nil
}

func sync_cluster_nodes(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	//sync the slu status for now
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()

	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, syc_functions["sync_nodes"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)

	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error syncing the nodes for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	return true, nil
}

func syncSlus(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	//sync the slu status for now
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()

	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, syc_functions["sync_slus"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)

	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error syncing the slus for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	return true, nil
}

func sync_cluster_storage_entities(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	// Sync the node details in cluster
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()

	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, syc_functions["sync_storages"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)

	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error syncing the storage entities of cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	return true, nil
}

func sync_block_devices(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	// Sync the node details in cluster
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()
	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, syc_functions["sync_block_devices"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)

	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error syncing the block devices for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	return true, nil
}
