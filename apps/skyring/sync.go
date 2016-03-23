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
	"fmt"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/monitoring"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"strconv"
)

var (
	syc_functions = map[string]string{
		"sync_slus": "SyncStorageLogicalUnitParams",
	}
)

func (a *App) SyncClusterDetails() {
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
		provider := a.GetProviderFromClusterId(ctxt, cluster.ClusterId)
		if provider == nil {
			logger.Get().Error("%s-Error getting provider for the cluster: %s", ctxt, cluster.Name)
			continue
		}

		// Sync the cluster status
		if ok, err := sync_cluster_status(ctxt, cluster, provider); err != nil || !ok {
			logger.Get().Error("%s-Error updating status for cluster: %s", ctxt, cluster.Name)
		}

		// Sync the cluster status
		if ok, err := syncSlus(ctxt, cluster, provider); err != nil || !ok {
			logger.Get().Error("%s-Error syncing slus: %s", ctxt, cluster.Name)
		}

		// TODO:: Sync the nodes status
		if ok, err := sync_cluster_nodes(ctxt, cluster, provider); err != nil && !ok {
			logger.Get().Error("%s-Error syncing node details for cluster: %s. error: %v", ctxt, cluster.Name, err)
		}
		// TODO:: Sync the storage entities of the cluster
		if ok, err := sync_cluster_storage_entities(ctxt, cluster, provider); err != nil && !ok {
			logger.Get().Error("%s-Error syncing storage entities for cluster: %s. error: %v", ctxt, cluster.Name, err)
		}
	}
}

func sync_cluster_status(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()
	err = provider.Client.Call(provider.Name+".GetClusterStatus", models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}}, &result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("i%s-Error getting status for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}
	clusterStatus, err := strconv.Atoi(string(result.Data.Result))
	if err != nil {
		logger.Get().Error("%s-Error getting status for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	// Set the cluster status
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_CLUSTERS)
	logger.Get().Info("Updating the status of the cluster: %s to %d", cluster.Name, clusterStatus)
	if err := coll.Update(bson.M{"clusterid": cluster.ClusterId}, bson.M{"$set": bson.M{"status": clusterStatus}}); err != nil {
		logger.Get().Error("%s-Error updating status for cluster: %s. error:%v", ctxt, cluster.Name, err)
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
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}},
		&result)

	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error syncing the slus for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	return true, nil
}

func sync_cluster_nodes(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	// TODO: Get the list of nodes from provider and add the new nodes to DB after comparison
	// with fetched nodes from DB
	return true, nil
}

func SyncNodeUtilizations(params map[string]interface{}) {
	ctxt, ctxtOk := params["ctxt"].(string)
	if !ctxtOk {
		logger.Get().Error("Failed to fetch context")
		return
	}
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var nodes []models.Node
	if err := coll.Find(bson.M{"state": models.NODE_STATE_ACTIVE}).All(&nodes); err != nil {
		if err == mgo.ErrNotFound {
			return
		}
		logger.Get().Warning("%s - Failed to fetch nodes in active state.Error %v", ctxt, err)
	}
	for _, node := range nodes {
		/*
			Get memory usage percentage
		*/
		resource_name := fmt.Sprintf("%s.%s", monitoring.MEMORY, monitoring.USAGE_PERCENTAGE)
		count := 0
		memory_usage_percent := FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)

		/*
			Get cpu user utilization
		*/
		var resource_name_error error
		resource_name, resource_name_error = GetMonitoringManager().GetResourceName(map[string]interface{}{
			"resource_name": monitoring.CPU_USER,
		})
		var cpu_user float64
		if resource_name_error == nil {
			cpu_user = FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)
		} else {
			logger.Get().Warning("%s - Failed to fetch cpu statistics from %v.Error %v", ctxt, node.Hostname, resource_name_error)
		}

		coll = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
		if coll.Update(
			bson.M{"nodeid": node.NodeId},
			bson.M{"$set": bson.M{
				"memorypercentageusage": memory_usage_percent,
				"cpupercentageusage":    cpu_user,
			}}); err != nil {
			logger.Get().Warning("%s - Failed to update memory and cpu utilizations of node %v to db.Error %v", ctxt, node.Hostname, err)
		}
	}
}

func sync_cluster_storage_entities(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE)

	// Get the list of storage entities from DB
	var fetchedStorages models.Storages
	if err := coll.Find(bson.M{"clusterid": cluster.ClusterId}).All(&fetchedStorages); err != nil {
		logger.Get().Error("%s-Error getting the storage entities for cluster: %v from DB. error: %v", ctxt, cluster.ClusterId, err)
		return false, err
	}

	// Get the list of storages from cluster
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()
	err = provider.Client.Call(provider.Name+".GetStorages", models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}}, &result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error getting storage details for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	} else {
		var storages []models.AddStorageRequest
		if err := json.Unmarshal(result.Data.Result, &storages); err != nil {
			logger.Get().Error("%s-Error parsing result from provider for storages list of cluster: %s. error: %v", ctxt, cluster.Name, err)
			return false, err
		}
		// Insert/update storages
		for _, storage := range storages {
			// Check if the pool already exists, if so update else insert
			if !storage_in_list(ctxt, fetchedStorages, storage.Name) {
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
					logger.Get().Error("%s-Error creating id for the new storage entity: %s. error: %v", ctxt, storage.Name, err)
					return false, err
				}
				entity.StorageId = *uuid
				if err := coll.Insert(entity); err != nil {
					logger.Get().Error("%s-Error adding storage:%s to DB. error: %v", ctxt, storage.Name, err)
					return false, err
				}
				logger.Get().Info("%s-Added the new storage entity: %s", ctxt, storage.Name)
			} else {
				// Update
				if err := coll.Update(
					bson.M{"name": storage.Name},
					bson.M{"$set": bson.M{
						"options":       storage.Options,
						"quota_enabled": storage.QuotaEnabled,
						"quota_params":  storage.QuotaParams,
					}}); err != nil {
					logger.Get().Error("%s-Error updating the storage entity: %s. error: %v", ctxt, storage.Name, err)
					return false, err
				}
				logger.Get().Info("%s-Updated details of storage entity: %s", ctxt, storage.Name)
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
					logger.Get().Error("%s-Error removing the storage: %s. error: %v", ctxt, fetchedStorage.Name, err)
				}
			}
		}
	}

	return true, nil
}

func storage_in_list(ctxt string, fetchedStorages models.Storages, name string) bool {
	for _, storage := range fetchedStorages {
		if storage.Name == name {
			return true
		}
	}
	return false
}
