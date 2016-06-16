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
	"github.com/skyrings/skyring-common/monitoring"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	sync_functions = map[string]string{
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
		logger.Get().Info(fmt.Sprintf("Started syncing cluster: %s", cluster.Name))
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
			// Re-set the cluster state to active
			if err := coll.Update(
				bson.M{"clusterid": cluster.ClusterId},
				bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
				logger.Get().Debug("%s-Failed to set state set to active for cluster: %v", ctxt, cluster.ClusterId)
			}
			continue
		}

		// Sync the cluster status
		logger.Get().Debug("Syncing status of cluster: %s", cluster.Name)
		if ok, err := sync_cluster_status(ctxt, cluster, provider); err != nil || !ok {
			// Re-set the cluster state to active
			if err := coll.Update(
				bson.M{"clusterid": cluster.ClusterId},
				bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
				logger.Get().Debug("%s-Failed to set state set to active for cluster: %v", ctxt, cluster.ClusterId)
			}
			logger.Get().Error("%s-Error updating status for cluster: %s", ctxt, cluster.Name)
			continue
		}

		// Sync the cluster nodes
		logger.Get().Debug("Syncing nodes of cluster: %s", cluster.Name)
		if ok, err := sync_cluster_nodes(ctxt, cluster, provider); err != nil || !ok {
			// Re-set the cluster state to active
			if err := coll.Update(
				bson.M{"clusterid": cluster.ClusterId},
				bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
				logger.Get().Debug("%s-Failed to set state set to active for cluster: %v", ctxt, cluster.ClusterId)
			}
			logger.Get().Error("%s-Error syncing storage nodes for cluster: %s", ctxt, cluster.Name)
			continue
		}

		// Sync the cluster status
		logger.Get().Debug("Syncing SLUs of cluster: %s", cluster.Name)
		if ok, err := syncSlus(ctxt, cluster, provider); err != nil || !ok {
			// Re-set the cluster state to active
			if err := coll.Update(
				bson.M{"clusterid": cluster.ClusterId},
				bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
				logger.Get().Debug("%s-Failed to set state set to active for cluster: %v", ctxt, cluster.ClusterId)
			}
			logger.Get().Error("%s-Error syncing slus: %s", ctxt, cluster.Name)
			continue
		}

		// Sync the storage entities of the cluster
		logger.Get().Debug("Syncing storages of cluster: %s", cluster.Name)
		if ok, err := sync_cluster_storage_entities(ctxt, cluster, provider); err != nil || !ok {
			// Re-set the cluster state to active
			if err := coll.Update(
				bson.M{"clusterid": cluster.ClusterId},
				bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
				logger.Get().Debug("%s-Failed to set state set to active for cluster: %v", ctxt, cluster.ClusterId)
			}
			logger.Get().Error("%s-Error syncing storage entities for cluster: %s. error: %v", ctxt, cluster.Name, err)
			continue
		}
		// Sync block devices
		logger.Get().Debug("Syncing block devices of cluster: %s", cluster.Name)
		if ok, err := sync_block_devices(ctxt, cluster, provider); err != nil || !ok {
			// Re-set the cluster state to active
			if err := coll.Update(
				bson.M{"clusterid": cluster.ClusterId},
				bson.M{"$set": bson.M{"state": models.CLUSTER_STATE_ACTIVE}}); err != nil {
				logger.Get().Debug("%s-Failed to set state set to active for cluster: %v", ctxt, cluster.ClusterId)
			}
			logger.Get().Error("%s-Error syncing block devices for cluster: %s. error: %v", ctxt, cluster.Name, err)
			continue
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
		fmt.Sprintf("%s.%s", provider.Name, sync_functions["cluster_status"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)
	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error getting status for cluster: %s. error:%v", ctxt, cluster.Name, err)
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
	logger.Get().Info("%s-Updating the status of the cluster: %s to %d", ctxt, cluster.Name, clusterStatus)
	if err := coll.Update(bson.M{"clusterid": cluster.ClusterId}, bson.M{"$set": bson.M{"status": clusterStatus}}); err != nil {
		logger.Get().Error("%s-Error updating status for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	return true, nil
}

func sync_cluster_nodes(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	//sync the slu status for now
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()

	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, sync_functions["sync_nodes"]),
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
		provider.Name, sync_functions["sync_slus"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)

	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error syncing the slus for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	return true, nil
}

func SyncNodeUtilizations(params map[string]interface{}) {
	var disk_writes float64
	var disk_reads float64
	var interface_rx float64
	var interface_tx float64

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
	time_stamp_str := strconv.FormatInt(time.Now().Unix(), 10)

	var nodeNames []string
	for _, node := range nodes {
		if node.State == models.NODE_STATE_ACTIVE {
			nodeNames = append(nodeNames, node.Hostname)
		}
	}

	if len(nodeNames) == 0 {
		return
	}

	var err error

	for _, node := range nodes {
		table_name := fmt.Sprintf("%s.%s.", conf.SystemConfig.TimeSeriesDBConfig.CollectionName, strings.Replace(node.Hostname, ".", "_", -1))
		/*
			Node wise storage utilisation
		*/
		var storageTotal int64
		var storageUsed int64

		collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_LOGICAL_UNITS)
		var slus []models.StorageLogicalUnit
		if err := collection.Find(bson.M{"nodeid": node.NodeId}).All(&slus); err != nil {
			if err != mgo.ErrNotFound {
				logger.Get().Error("%s - Could not fetch slus of node %v.Error %v", ctxt, node.Hostname, err)
				return
			}
		}
		for _, slu := range slus {
			storageTotal = storageTotal + slu.Usage.Total
			storageUsed = storageUsed + slu.Usage.Used
		}
		var storageUsagePercent float64
		if storageTotal != 0 {
			storageUsagePercent = float64(storageUsed*100) / float64(storageTotal)
		}
		UpdateMetricToTimeSeriesDb(ctxt, storageUsagePercent, time_stamp_str, fmt.Sprintf("%s%s.%s", table_name, monitoring.STORAGE_UTILIZATION, monitoring.PERCENT_USED))
		UpdateMetricToTimeSeriesDb(ctxt, float64(storageUsed), time_stamp_str, fmt.Sprintf("%s%s.%s", table_name, monitoring.STORAGE_UTILIZATION, monitoring.USED_SPACE))
		UpdateMetricToTimeSeriesDb(ctxt, float64(storageTotal), time_stamp_str, fmt.Sprintf("%s%s.%s", table_name, monitoring.STORAGE_UTILIZATION, monitoring.TOTAL_SPACE))

		/*
			Get memory statistics
		*/
		resource_name := fmt.Sprintf("%s.%s", monitoring.MEMORY, monitoring.USAGE_PERCENTAGE)
		count := 0
		memory_usage_percent := FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)

		//Memory total
		var memory_total float64
		resource_name, resourceNameError := GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": monitoring.AGGREGATION + monitoring.MEMORY})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v .Err %v", ctxt, monitoring.AGGREGATION+monitoring.MEMORY, node.Hostname, resourceNameError)
		} else {
			memory_total = FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)
		}

		//Memory used
		var memory_used float64
		resource_name = fmt.Sprintf("%s.%s-%s", monitoring.MEMORY, monitoring.MEMORY, monitoring.USED)
		memory_used = FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)

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

		/*
			Get swap used
		*/
		var swap_used float64
		resource_name = fmt.Sprintf("%s.%s-%s", monitoring.SWAP, monitoring.SWAP, monitoring.USED)
		swap_used = FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)

		resource_name = fmt.Sprintf("%s.%s", monitoring.SWAP, monitoring.USAGE_PERCENTAGE)
		swap_usage_percent := FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)

		var swap_total float64
		resource_name, resourceNameError = GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": monitoring.AGGREGATION + monitoring.SWAP})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v .Err %v", ctxt, monitoring.AGGREGATION+monitoring.SWAP, node.Hostname, resourceNameError)
		} else {
			swap_total = FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)
		}

		//Network used
		var nwUsed float64
		resource_name, resourceNameError = GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": monitoring.AVERAGE + monitoring.INTERFACE + monitoring.USED})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v .Err %v", ctxt, monitoring.AVERAGE+monitoring.INTERFACE+monitoring.USED, node.Hostname, resourceNameError)
		} else {
			nwUsed = FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)
		}

		var nwBandwidth float64
		resource_name, resourceNameError = GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": monitoring.AVERAGE + monitoring.INTERFACE + monitoring.TOTAL})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v .Err %v", ctxt, monitoring.AVERAGE+monitoring.INTERFACE+monitoring.TOTAL, node.Hostname, resourceNameError)
		} else {
			nwBandwidth = FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)
		}

		var nwPercentUsage float64
		resource_name, resourceNameError = GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": monitoring.AVERAGE + monitoring.INTERFACE + monitoring.PERCENT})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v .Err %v", ctxt, monitoring.AVERAGE+monitoring.INTERFACE+monitoring.PERCENT, node.Hostname, resourceNameError)
		} else {
			nwPercentUsage = FetchStatFromGraphite(ctxt, node.Hostname, resource_name, &count)
		}

		coll = sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
		utilizations := map[string]models.Utilization{
			"memoryusage": {
				Used:        int64(memory_used),
				Total:       int64(memory_total),
				PercentUsed: memory_usage_percent,
				UpdatedAt:   time.Now().String(),
			},
			"cpuusage": {
				Used:        int64(cpu_user),
				Total:       int64(100),
				PercentUsed: cpu_user,
				UpdatedAt:   time.Now().String(),
			},
			"storageusage": {
				Used:        storageUsed,
				Total:       storageTotal,
				PercentUsed: storageUsagePercent,
				UpdatedAt:   time.Now().String(),
			},
			"swapusage": {
				Used:        int64(swap_used),
				Total:       int64(swap_total),
				PercentUsed: swap_usage_percent,
				UpdatedAt:   time.Now().String(),
			},
			"networkusage": {
				Used:        int64(nwUsed),
				Total:       int64(nwBandwidth),
				PercentUsed: nwPercentUsage,
				UpdatedAt:   time.Now().String(),
			},
		}

		if coll.Update(
			bson.M{"nodeid": node.NodeId},
			bson.M{"$set": bson.M{"utilizations": utilizations}}); err != nil {
			logger.Get().Warning("%s - Failed to update memory and cpu utilizations of node %v to db.Error %v", ctxt, node.Hostname, err)
		}

		// Aggregate disk read
		resourcePrefix := monitoring.AGGREGATION + monitoring.DISK
		resource_name, resourceNameError = GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": resourcePrefix + monitoring.READ})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v .Err %v", ctxt, resource_name, node.Hostname, resourceNameError)
		} else {
			disk_reads_count := 1
			disk_reads = FetchAggregatedStatsFromGraphite(ctxt, node.Hostname, resource_name, &disk_reads_count, []string{})
		}

		// Aggregate disk write
		resource_name, resourceNameError = GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": resourcePrefix + monitoring.WRITE})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v.Err %v", ctxt, resource_name, node.Hostname, resourceNameError)
		} else {
			disk_writes_count := 1
			disk_writes = FetchAggregatedStatsFromGraphite(ctxt, node.Hostname, resource_name, &disk_writes_count, []string{})
		}
		UpdateMetricToTimeSeriesDb(ctxt, disk_reads+disk_writes, time_stamp_str, fmt.Sprintf("%s%s-%s_%s", table_name, monitoring.DISK, monitoring.READ, monitoring.WRITE))

		// Aggregate interface rx
		resourcePrefix = monitoring.AGGREGATION + monitoring.INTERFACE + monitoring.OCTETS
		resource_name, resourceNameError = GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": resourcePrefix + monitoring.RX})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v.Err %v", ctxt, resourcePrefix+monitoring.RX, node.Hostname, resourceNameError)
		} else {
			interface_rx_count := 1
			interface_rx = FetchAggregatedStatsFromGraphite(ctxt, node.Hostname, resource_name, &interface_rx_count, []string{monitoring.LOOP_BACK_INTERFACE})
		}

		// Aggregate interface tx
		resource_name, resourceNameError = GetMonitoringManager().GetResourceName(map[string]interface{}{"resource_name": resourcePrefix + monitoring.TX})
		if resourceNameError != nil {
			logger.Get().Warning("%s - Failed to fetch resource name of %v for %v.Err %v", ctxt, resource_name, node.Hostname, resourceNameError)
		} else {
			interface_tx_count := 1
			interface_tx = FetchAggregatedStatsFromGraphite(ctxt, node.Hostname, resource_name, &interface_tx_count, []string{monitoring.LOOP_BACK_INTERFACE})
		}

		UpdateMetricToTimeSeriesDb(ctxt, interface_rx+interface_tx, time_stamp_str, fmt.Sprintf("%s%s-%s_%s", table_name, monitoring.INTERFACE, monitoring.RX, monitoring.TX))
	}
}

func sync_cluster_storage_entities(ctxt string, cluster models.Cluster, provider *Provider) (bool, error) {
	// Sync the node details in cluster
	var result models.RpcResponse
	vars := make(map[string]string)
	vars["cluster-id"] = cluster.ClusterId.String()

	err = provider.Client.Call(fmt.Sprintf("%s.%s",
		provider.Name, sync_functions["sync_storages"]),
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
		provider.Name, sync_functions["sync_block_devices"]),
		models.RpcRequest{RpcRequestVars: vars, RpcRequestData: []byte{}, RpcRequestContext: ctxt},
		&result)

	if err != nil || result.Status.StatusCode != http.StatusOK {
		logger.Get().Error("%s-Error syncing the block devices for cluster: %s. error:%v", ctxt, cluster.Name, err)
		return false, err
	}

	return true, nil
}
