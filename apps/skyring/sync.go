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
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"net/http"
)

func (a *App) SyncClusterDetails() {
	// Get the list of cluster
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.Clusters
	if err := coll.Find(nil).All(&clusters); err != nil {
		logger.Get().Error("Error getting the clusters list. Unable to sync details. error: %v", err)
		return
	}
	for _, cluster := range clusters {
		provider := a.getProviderFromClusterId(cluster.ClusterId)
		if provider == nil {
			logger.Get().Error("Error getting provider for the cluster: %s", cluster.Name)
			continue
		}

		// Sync the cluster status
		if ok, err := sync_cluster_status(cluster, provider); err != nil || !ok {
			logger.Get().Error("Error updating status for cluster: %s", cluster.Name)
		}
		// TODO:: Sync the nodes status
		if ok, err := sync_cluster_nodes(cluster, provider); err != nil && !ok {
			logger.Get().Error("Error syncing node details for cluster: %s. error: %v", cluster.Name, err)
		}
		// TODO:: Sync the storage entities of the cluster
		if ok, err := sync_cluster_storage_entities(cluster, provider); err != nil && !ok {
			logger.Get().Error("Error syncing storage entities for cluster: %s. error: %v", cluster.Name, err)
		}
	}
}

func (a *App) SyncClusterStatusDetails() {
	// Get the list of cluster
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	var clusters models.Clusters
	if err := coll.Find(nil).All(&clusters); err != nil {
		logger.Get().Error("Error getting the clusters list. Unable to sync details. error: %v", err)
		return
	}
	for _, cluster := range clusters {
		provider := a.getProviderFromClusterId(cluster.ClusterId)
		if provider == nil {
			logger.Get().Error("Error getting provider for the cluster: %s", cluster.Name)
			continue
		}

		// Sync the cluster status
		if ok, err := sync_cluster_status(cluster, provider); err != nil || !ok {
			logger.Get().Error("Error updating status for cluster: %s", cluster.Name)
		}
		// TODO:: Sync the nodes status
		if ok, err := sync_cluster_nodes(cluster, provider); err != nil && !ok {
			logger.Get().Error("Error syncing node details for cluster: %s. error: %v", cluster.Name, err)
		}
	}
}

func sync_cluster_status(cluster models.Cluster, provider *Provider) (bool, error) {
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()
	err = provider.Client.Call(provider.Name+".GetClusterStatus", models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}}, &result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("Error getting status for cluster: %s. error:%v", cluster.Name, err)
		return false, err
	} else {
		statusMsg := result.Status.StatusMessage
		sessionCopy := db.GetDatastore().Copy()
		defer sessionCopy.Close()
		coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
		var clusterStatus int
		switch statusMsg {
		case models.STATUS_OK:
			clusterStatus = models.CLUSTER_STATUS_OK
		case models.STATUS_WARN:
			clusterStatus = models.CLUSTER_STATUS_WARN
		case models.STATUS_ERR:
			clusterStatus = models.CLUSTER_STATUS_ERROR
		}
		// Set the cluster status
		logger.Get().Info("Updating the status of the cluster: %s to %d", cluster.Name, clusterStatus)
		if err := coll.Update(bson.M{"clusterid": cluster.ClusterId}, bson.M{"$set": bson.M{"status": clusterStatus}}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func sync_cluster_nodes(cluster models.Cluster, provider *Provider) (bool, error) {
	// TODO: Get the list of nodes from provider and add the new nodes to DB after comparison
	// with fetched nodes from DB
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes models.Nodes
	if err := coll.Find(bson.M{"clusterid": cluster.ClusterId}).All(&nodes); err != nil {
		logger.Get().Error("Error getting the nodes for the cluster: %v. error: %v", cluster.ClusterId, err)
		return false, err
	}
	for _, node := range nodes {
		if GetCoreNodeManager().SyncNodeStatus(node.Hostname); err != nil {
			logger.Get().Error("Error syncing the status for node: %s. error: %v", node.Hostname, err)
		}
	}

	return true, nil
}

func sync_cluster_storage_entities(cluster models.Cluster, provider *Provider) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)

	// Get the list of storage entities from DB
	var fetchedStorages models.Storages
	if err := coll.Find(bson.M{"clusterid": cluster.ClusterId}).All(&fetchedStorages); err != nil {
		logger.Get().Error("Error getting the storages from DB: %v", err)
		return false, err
	}

	// Get the list of storages from cluster
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()
	err = provider.Client.Call(provider.Name+".GetStorages", models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}}, &result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("Error getting storage details for cluster: %s. error:%v", cluster.Name, err)
		return false, err
	} else {
		var storages []models.AddStorageRequest
		if err := json.Unmarshal(result.Data.Result, &storages); err != nil {
			logger.Get().Error("Error parsing result from provider: %v", err)
			return false, err
		}
		// Insert/update storages
		for _, storage := range storages {
			// Check if the pool already exists, if so update else insert
			if !storage_in_list(fetchedStorages, storage.Name) {
				// Not found, insert
				entity := models.Storage{
					ClusterId:    cluster.ClusterId,
					Name:         storage.Name,
					Type:         storage.Type,
					Replicas:     storage.Replicas,
					QuotaEnabled: storage.QuotaEnabled,
					QuotaParams:  storage.QuotaParams,
					Options:      storage.Options,
				}
				uuid, err := uuid.New()
				if err != nil {
					logger.Get().Error("Error creating id for the new storage entity: %s", storage.Name)
					return false, err
				}
				entity.StorageId = *uuid
				if err := coll.Insert(entity); err != nil {
					logger.Get().Error("Error adding storage:%s to DB", storage.Name)
					return false, err
				}
				logger.Get().Info("Added the new storage entity: %s", storage.Name)
			} else {
				// Update
				if err := coll.Update(
					bson.M{"name": storage.Name},
					bson.M{"$set": bson.M{
						"options":       storage.Options,
						"quota_enabled": storage.QuotaEnabled,
						"quota_params":  storage.QuotaParams,
					}}); err != nil {
					logger.Get().Error("Error updating the storage entity: %s", storage.Name)
					return false, err
				}
				logger.Get().Info("Updated details of storage entity: %s", storage.Name)
			}
		}
		// Delete the un-wanted storages
		for _, fetchedStorage := range fetchedStorages {
			found := false
			for _, storage := range storages {
				if storage.Name == fetchedStorage.Name {
					found = true
					break
				}
			}
			if !found {
				if err := coll.Remove(bson.M{"storageid": fetchedStorage.StorageId}); err != nil {
					logger.Get().Error("Error removing the storage: %s", fetchedStorage.Name)
				}
			}
		}
	}

	return true, nil
}

func storage_in_list(fetchedStorages models.Storages, name string) bool {
	for _, storage := range fetchedStorages {
		if storage.Name == name {
			return true
		}
	}
	return false
}
