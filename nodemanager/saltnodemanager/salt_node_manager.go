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
package saltnodemanager

import (
	"errors"
	"fmt"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/monitoring"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring/backend/salt"
	"github.com/skyrings/skyring/nodemanager"
	"gopkg.in/mgo.v2/bson"
	"io"
	"net"
)

const (
	NodeManagerName = "SaltNodeManager"
)

var (
	salt_backend = salt.New()
)

type SaltNodeManager struct {
}

func init() {
	nodemanager.RegisterNodeManager(NodeManagerName, func(config io.Reader) (nodemanager.NodeManagerInterface, error) {
		return NewSaltNodeManager(config)
	})
}

func NewSaltNodeManager(config io.Reader) (*SaltNodeManager, error) {
	return &SaltNodeManager{}, nil
}

func (a SaltNodeManager) AcceptNode(node string, fingerprint string, ctxt string) (bool, error) {
	if status, err := salt_backend.AcceptNode(node, fingerprint, false, ctxt); err != nil {
		return false, err
	} else if !status {
		return false, errors.New(fmt.Sprintf("Unable to accept the node: %s", node))
	} else {
		return true, nil

	}
}

func (a SaltNodeManager) AddNode(master string, node string, port uint, fingerprint string, username string, password string, ctxt string) (bool, error) {
	if status, err := salt_backend.AddNode(master, node, port, fingerprint, username, password, ctxt); err != nil {
		return false, err
	} else if !status {
		return false, errors.New(fmt.Sprintf("Unable to add the node: %s", node))
	} else {
		return true, nil
	}
}

func SetupSkynetService(hostname string, ctxt string) error {
	if ok, err := salt_backend.SetupSkynetService(hostname, ctxt); err != nil || !ok {
		logger.Get().Error("%s-could not setup skynet service on the node: %s", ctxt, hostname)
		return errors.New(fmt.Sprintf("could not setup skynet service on the node: %s", hostname))
	}
	return nil
}

func GetStorageNodeInstance(hostname string, sProfiles []models.StorageProfile, ctxt string) (*models.Node, bool) {
	var storage_node models.Node
	storage_node.Hostname = hostname
	storage_node.Enabled = true
	storage_node.NodeId, _ = salt_backend.GetNodeID(hostname, ctxt)
	networkInfo, err := salt_backend.GetNodeNetwork(hostname, ctxt)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting network details for node: %s. error: %v", ctxt, hostname, err))
		return nil, false
	}
	storage_node.NetworkInfo = networkInfo
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error looking up node IP for: %s. error: %v", ctxt, hostname, err))
		return nil, false
	}
	storage_node.ManagementIP4 = addrs[0]
	ok, err := salt_backend.NodeUp(hostname, ctxt)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting status of node: %s. error: %v", ctxt, hostname, err))
		return nil, false
	}
	if ok {
		storage_node.Status = models.NODE_STATUS_OK
	} else {
		storage_node.Status = models.NODE_STATUS_ERROR
	}
	disks, err := salt_backend.GetNodeDisk(hostname, ctxt)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting disk details for node: %s. error: %v", ctxt, hostname, err))
		return nil, false
	}
	for _, disk := range disks {
		dId, err := uuid.New()
		if err != nil {
			logger.Get().Error(fmt.Sprintf("%s-Unable to generate uuid for disk : %s. error: %v", ctxt, disk.DevName, err))
			return nil, false
		}
		disk.DiskId = *dId
		applyStorageProfile(&disk, sProfiles)
		storage_node.StorageDisks = append(storage_node.StorageDisks, disk)
	}

	cpus, err := salt_backend.GetNodeCpu(hostname, ctxt)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting cpu details for node: %s. error: %v", ctxt, hostname, err))
		return nil, false
	}
	for _, cpu := range cpus {
		storage_node.CPUs = append(storage_node.CPUs, cpu)
	}

	osInfo, err := salt_backend.GetNodeOs(hostname, ctxt)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting os details for node: %s", ctxt, hostname))
		return nil, false
	}
	storage_node.OS = osInfo

	memoryInfo, err := salt_backend.GetNodeMemory(hostname, ctxt)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting memory details for node: %s", ctxt, hostname))
		return nil, false
	}
	storage_node.Memory = memoryInfo

	if !storage_node.NodeId.IsZero() && len(storage_node.NetworkInfo.Subnet) != 0 && len(storage_node.StorageDisks) != 0 {
		return &storage_node, true
	} else {
		return nil, false
	}
}

func (a SaltNodeManager) IsNodeUp(hostname string, ctxt string) (bool, error) {
	ok, err := salt_backend.NodeUp(hostname, ctxt)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("%s-Error getting status of node: %s. error: %v", ctxt, hostname, err))
		return false, nil
	}
	return ok, nil
}

func (a SaltNodeManager) NodeUptime(hostname string, ctxt string) (string, error) {
	uptime, err := salt_backend.NodeUptime(hostname, ctxt)
	if err != nil {
		return "", err
	}
	return uptime, nil
}

func (a SaltNodeManager) GetUnmanagedNodes(ctxt string) (*models.UnmanagedNodes, error) {
	if nodes, err := salt_backend.GetNodes(ctxt); err != nil {
		return nil, err
	} else {
		var retNodes models.UnmanagedNodes
		for _, node := range nodes.Unmanage {
			var retNode models.UnmanagedNode
			retNode.Name = node.Name
			retNode.SaltFingerprint = node.Fingerprint
			retNodes = append(retNodes, retNode)
		}
		return &retNodes, nil
	}
}

func (a SaltNodeManager) SyncStorageDisks(node string, sProfiles []models.StorageProfile, ctxt string) (bool, error) {
	disks, err := salt_backend.GetNodeDisk(node, ctxt)
	if err != nil {
		return false, err
	}
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	var storage_node models.Node
	var updated_disks []models.Disk

	if err := coll.Find(bson.M{"hostname": node}).One(&storage_node); err != nil {
		logger.Get().Error("%s-Error updating the disk details for node: %s. error: %v", ctxt, node, err)
		return false, err
	}
	var present bool
	for _, disk := range disks {
		present = false
		for _, stored_disk := range storage_node.StorageDisks {
			if disk.DevName == stored_disk.DevName {
				present = true
				updated_disks = append(updated_disks, stored_disk)
				break
			}
		}
		if !present {
			dId, err := uuid.New()
			if err != nil {
				logger.Get().Error(fmt.Sprintf("%s-Unable to generate uuid for disk : %s. error: %v", ctxt, disk.DevName, err))
				return false, err
			}
			disk.DiskId = *dId
			applyStorageProfile(&disk, sProfiles)
			updated_disks = append(updated_disks, disk)
		}
	}
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"storagedisks": updated_disks}}); err != nil {
		logger.Get().Error("%s-Error updating the disk details for node: %s. error: %v", ctxt, node, err)
		return false, err
	}
	return true, nil
}

func (a SaltNodeManager) DisableNode(node string, ctxt string) (bool, error) {
	if ok, err := salt_backend.DisableService(node, "collectd", true, ctxt); err != nil || !ok {
		logger.Get().Error(fmt.Sprintf("%s-Error disabling services on node: %s. error: %v", ctxt, node, err))
		return false, err
	}

	// Disable any POST actions for participating nodes
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"enabled": false}}); err != nil {
		logger.Get().Error("%s-Error updating managed state of node: %s. error: %v", ctxt, node, err)
		return false, err
	}

	return true, nil
}

func (a SaltNodeManager) EnableNode(node string, ctxt string) (bool, error) {
	if ok, err := salt_backend.EnableService(node, "collectd", true, ctxt); err != nil || !ok {
		logger.Get().Error("%s-Error enabling services on node: %s. error: %v", ctxt, node, err)
		return false, err
	}

	// Enable any POST actions for node
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"enabled": true}}); err != nil {
		logger.Get().Error("%s-Error updating manage state of node: %s. error: %v", ctxt, node, err)
		return false, err
	}

	return true, nil
}

func (a SaltNodeManager) RemoveNode(node string, ctxt string) (bool, error) {
	if ok, err := salt_backend.DisableService(node, "collectd", true, ctxt); err != nil || !ok {
		return false, err
	}

	if ok, err := salt_backend.IgnoreNode(node, ctxt); err != nil || !ok {
		return false, err
	}

	return true, nil
}

func (a SaltNodeManager) IgnoreNode(node string, ctxt string) (bool, error) {
	if ok, err := salt_backend.IgnoreNode(node, ctxt); err != nil || !ok {
		logger.Get().Error(fmt.Sprintf("%s-Error rejecting node: %s. error: %v", ctxt, node, err))
		return false, err
	}

	return true, nil
}

func (a SaltNodeManager) SetUpMonitoring(node string, master string, ctxt string) (map[string]interface{}, error) {
	// The plugins to be setup includes the editable plugins and also the write plugin
	return a.EnforceMonitoring(
		append(monitoring.SupportedMonitoringPlugins, monitoring.MonitoringWritePlugin),
		[]string{node},
		master,
		monitoring.GetDefaultThresholdValues(),
		ctxt)
}

func (a SaltNodeManager) EnforceMonitoring(plugin_names []string, nodes []string, master string, plugins []monitoring.Plugin, ctxt string) (map[string]interface{}, error) {
	failed_nodes, err := salt_backend.AddMonitoringPlugin(plugin_names, nodes, master, monitoring.ToSaltPillarCompat(plugins), ctxt)
	return failed_nodes, err
}

func (a SaltNodeManager) UpdateMonitoringConfiguration(nodes []string, config []monitoring.Plugin, ctxt string) (map[string]string, error) {
	failed_nodes, err := salt_backend.UpdateMonitoringConfiguration(nodes, config, ctxt)
	return failed_nodes, err
}

func (a SaltNodeManager) EnableMonitoringPlugin(nodes []string, pluginName string, ctxt string) (map[string]string, error) {
	failed_nodes, err := salt_backend.EnableMonitoringPlugin(nodes, pluginName, ctxt)
	return failed_nodes, err
}

func (a SaltNodeManager) DisableMonitoringPlugin(nodes []string, pluginName string, ctxt string) (map[string]string, error) {
	failed_nodes, err := salt_backend.DisableMonitoringPlugin(nodes, pluginName, ctxt)
	return failed_nodes, err
}

func (a SaltNodeManager) RemoveMonitoringPlugin(nodes []string, pluginName string, ctxt string) (map[string]string, error) {
	failed_nodes, err := salt_backend.RemoveMonitoringPlugin(nodes, pluginName, ctxt)
	return failed_nodes, err
}

func (a SaltNodeManager) AddMonitoringPlugin(nodes []string, master string, plugin monitoring.Plugin, ctxt string) (map[string]interface{}, error) {
	failed_nodes, err := salt_backend.AddMonitoringPlugin([]string{plugin.Name}, nodes, "", monitoring.ToSaltPillarCompat([]monitoring.Plugin{plugin}), ctxt)
	return failed_nodes, err
}

func (a SaltNodeManager) SyncModules(node string, ctxt string) (bool, error) {
	if ok, err := salt_backend.SyncModules(node, ctxt); !ok || err != nil {
		return false, err
	}
	return true, nil
}

func applyStorageProfile(disk *models.Disk, sProfiles []models.StorageProfile) error {

	for _, sProfile := range sProfiles {

		//TODO check the speed

		//Check the disk type
		//Now only check whether the disk is SSD or not. REST are supported
		//once the API is avaialable
		diskType := sProfile.Rule.Type
		switch diskType {
		case models.SSD:
			if disk.SSD {
				disk.StorageProfile = sProfile.Name
				return nil
			}
		}
	}
	//Not Matched, so add to generic profile
	disk.StorageProfile = models.DefaultProfile3
	return nil
}

func GetFingerPrint(hostname string, ctxt string) (string, error) {
	fingerprint, err := salt_backend.GetFingerPrint(hostname, ctxt)
	return fingerprint, err
}
