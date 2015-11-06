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
	"github.com/skyrings/skyring/backend/salt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/event"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/nodemanager"
	"github.com/skyrings/skyring/tools/logger"
	"gopkg.in/mgo.v2/bson"
	"io"
	"net"
	"time"
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

func (a SaltNodeManager) AcceptNode(node string, fingerprint string) (*models.Node, error) {
	if ok, err := salt_backend.AcceptNode(node, fingerprint, false); err != nil || !ok {
		return nil, err
	} else {
		for count := 0; count < 60; count++ {
			time.Sleep(10 * time.Second)
			startedNodes := event.GetStartedNodes()
			for _, nodeName := range startedNodes {
				if nodeName == node {
					if retVal, ok := populateStorageNodeInstance(node); ok {
						return retVal, nil
					}
				}
			}
		}

	}

	return nil, errors.New("Unable to accept the node")
}

func (a SaltNodeManager) AddNode(master string, node string, port uint, fingerprint string, username string, password string) (*models.Node, error) {
	if ok, err := salt_backend.AddNode(master, node, port, fingerprint, username, password); err != nil || !ok {
		return nil, err
	} else {
		for count := 0; count < 60; count++ {
			time.Sleep(10 * time.Second)
			startedNodes := event.GetStartedNodes()
			for _, nodeName := range startedNodes {
				if nodeName == node {
					if retVal, ok := populateStorageNodeInstance(node); ok {
						return retVal, nil
					}
				}
			}
		}

	}

	return nil, errors.New("Unable to add the node")
}

func populateStorageNodeInstance(node string) (*models.Node, bool) {
	var storage_node models.Node
	storage_node.Hostname = node
	storage_node.Enabled = true
	storage_node.NodeId, _ = salt_backend.GetNodeID(node)
	networkInfo, err := salt_backend.GetNodeNetwork(node)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("Error getting network details for node: %s", node))
		return nil, false
	}
	storage_node.NetworkInfo.Subnet = networkInfo.Subnet
	storage_node.NetworkInfo.Ipv4 = networkInfo.IPv4
	storage_node.NetworkInfo.Ipv6 = networkInfo.IPv6
	addrs, err := net.LookupHost(node)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("Error looking up node IP for: %s", node))
		return nil, false
	}
	storage_node.ManagementIP4 = addrs[0]
	disks, err := salt_backend.GetNodeDisk(node)
	if err != nil {
		logger.Get().Error(fmt.Sprintf("Error getting disk details for node: %s", node))
		return nil, false
	}
	for _, disk := range disks {
		storage_node.StorageDisks = append(storage_node.StorageDisks, disk)
	}

	if !storage_node.NodeId.IsZero() && len(storage_node.NetworkInfo.Subnet) != 0 && len(storage_node.StorageDisks) != 0 {
		return &storage_node, true
	} else {
		return nil, false
	}
}

func (a SaltNodeManager) GetUnmanagedNodes() (*models.UnmanagedNodes, error) {
	if nodes, err := salt_backend.GetNodes(); err != nil {
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

func (a SaltNodeManager) SyncStorageDisks(node string) (bool, error) {
	disks, err := salt_backend.GetNodeDisk(node)
	if err != nil {
		return false, err
	}
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if len(disks) != 0 {
		if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"storage_disks": disks}}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (a SaltNodeManager) DisableNode(node string) (bool, error) {
	if ok, err := salt_backend.DisableService(node, "collectd", true); err != nil || !ok {
		logger.Get().Error(fmt.Sprintf("Error disabling services on node: %s, error: %v", node, err))
		return false, err
	}

	if ok, err := salt_backend.IgnoreNode(node); err != nil || !ok {
		logger.Get().Error(fmt.Sprintf("Error rejecting node: %s, error: %v", node, err))
		return false, err
	}

	// Disable any POST actions for participating nodes
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"enabled": false}}); err != nil {
		return false, err
	}

	return true, nil
}

func (a SaltNodeManager) EnableNode(node string) (bool, error) {
	nodes, err := salt_backend.GetNodes()
	if err != nil {
		logger.Get().Error(fmt.Sprintf("Error getting started nodes. error: %v", err))
		return false, err
	}

	fingerprint := ""
	for _, ignored_node := range nodes.Ignore {
		if ignored_node.Name == node {
			fingerprint = ignored_node.Fingerprint
			break
		}
	}

	if ok, err := salt_backend.AcceptNode(node, fingerprint, true); err != nil || !ok {
		logger.Get().Error(fmt.Sprintf("Error accepting the node:%s back. error: %v", node, err))
		return false, err
	}

	if ok, err := salt_backend.EnableService(node, "collectd", true); err != nil || !ok {
		logger.Get().Error(fmt.Sprintf("Error enabling services on the node: %s. error: %v", node, err))
		return false, err
	}

	// Enable any POST actions for participating nodes
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"enabled": true}}); err != nil {
		return false, err
	}

	return true, nil
}

func (a SaltNodeManager) RemoveNode(node string) (bool, error) {
	if ok, err := salt_backend.DisableService(node, "collectd", true); err != nil || !ok {
		return false, err
	}

	if ok, err := salt_backend.IgnoreNode(node); err != nil || !ok {
		return false, err
	}

	return true, nil
}
