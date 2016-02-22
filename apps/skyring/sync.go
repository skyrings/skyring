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
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"strconv"
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

func sync_cluster_status(cluster models.Cluster, provider *Provider) (bool, error) {
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()
	err = provider.Client.Call(provider.Name+".GetClusterStatus", models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}}, &result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("Error getting status for cluster: %s. error:%v", cluster.Name, err)
		return false, err
	}
	clusterStatus, err := strconv.Atoi(string(result.Data.Result))
	if err != nil {
		logger.Get().Error("Error getting status for cluster: %s. error:%v", cluster.Name, err)
		return false, err
	}

	// Set the cluster status
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	logger.Get().Info("Updating the status of the cluster: %s to %d", cluster.Name, clusterStatus)
	if err := coll.Update(bson.M{"clusterid": cluster.ClusterId}, bson.M{"$set": bson.M{"status": clusterStatus}}); err != nil {
		logger.Get().Error("Error updating status for cluster: %s. error:%v", cluster.Name, err)
		return false, err
	}

	return true, nil
}

func sync_cluster_nodes(cluster models.Cluster, provider *Provider) (bool, error) {
	// TODO: Get the list of nodes from provider and add the new nodes to DB after comparison
	// with fetched nodes from DB
	return true, nil
}

func sync_cluster_storage_entities(cluster models.Cluster, provider *Provider) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)

	// Get the list of storage entities from DB
	var fetchedStorages models.Storages
	if err := coll.Find(bson.M{"clusterid": cluster.ClusterId}).All(&fetchedStorages); err != nil {
		logger.Get().Error("Error getting the storage entities for cluster: %v from DB. error: %v", cluster.ClusterId, err)
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
			logger.Get().Error("Error parsing result from provider for storages list of cluster: %s. error: %v", cluster.Name, err)
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
					logger.Get().Error("Error creating id for the new storage entity: %s. error: %v", storage.Name, err)
					return false, err
				}
				entity.StorageId = *uuid
				if err := coll.Insert(entity); err != nil {
					logger.Get().Error("Error adding storage:%s to DB. error: %v", storage.Name, err)
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
					logger.Get().Error("Error updating the storage entity: %s. error: %v", storage.Name, err)
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
					logger.Get().Error("Error removing the storage: %s. error: %v", fetchedStorage.Name, err)
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
