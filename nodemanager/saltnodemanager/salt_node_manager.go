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
	"gopkg.in/mgo.v2/bson"
	"io"
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

func (a SaltNodeManager) AcceptNode(node string, fingerprint string) (*models.StorageNode, error) {
	if _, err := salt_backend.AcceptNode(node, fingerprint); err != nil {
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

func (a SaltNodeManager) AddNode(master string, node string, port uint, fingerprint string, username string, password string) (*models.StorageNode, error) {
	if _, err := salt_backend.AddNode(master, node, port, fingerprint, username, password); err != nil {
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

func populateStorageNodeInstance(node string) (*models.StorageNode, bool) {
	var storage_node models.StorageNode
	storage_node.Hostname = node
	storage_node.AdministrativeStatus = models.FREE
	storage_node.UUID, _ = salt_backend.GetNodeID(node)
	networkInfo, _ := salt_backend.GetNodeNetwork(node)
	storage_node.NetworkInfo.Subnet = networkInfo.Subnet
	storage_node.NetworkInfo.Ipv4 = networkInfo.IPv4
	storage_node.NetworkInfo.Ipv6 = networkInfo.IPv6
	storage_node.ManagementIp = networkInfo.IPv4[0]
	disks, _ := salt_backend.GetNodeDisk(node)
	for _, disk := range disks {
		var storageDisk models.StorageDisk
		storageDisk.Disk = disk
		storageDisk.AdministrativeStatus = models.FREE
		storage_node.StorageDisks = append(storage_node.StorageDisks, storageDisk)
	}

	if !storage_node.UUID.IsZero() && len(storage_node.NetworkInfo.Subnet) != 0 && len(storage_node.StorageDisks) != 0 {
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

func (a SaltNodeManager) DisableNode(node string) (bool, error) {
	fmt.Println("Stopping services on " + node)
	ok, err := salt_backend.StopAndDisableService(node, "collectd")
	if err != nil || !ok {
		return ok, err
	}

	fmt.Println("Disabling salt communication for " + node)
	ok, err = salt_backend.RejectNode(node)
	if err != nil || !ok {
		return ok, err
	}

	// Disable any POST actions for participating nodes
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"administrativestatus": models.UNMANAGED}}); err != nil {
		return false, err
	}

	return true, nil
}

func (a SaltNodeManager) EnableNode(node string) (bool, error) {
	fingerprint, err := salt_backend.GetRejectedFingerprint(node)
	fmt.Println(fingerprint)
	fmt.Println(err)
	if err != nil {
		return false, err
	}

	if _, err := salt_backend.AcceptRejectedNode(node, fingerprint); err != nil {
		return false, err
	}

	ok, err := salt_backend.EnableAndStartService(node, "collectd")
	if err != nil || !ok {
		return ok, err
	}
	fmt.Println(ok)
	fmt.Println(err)

	// Enable any POST actions for participating nodes
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Update(bson.M{"hostname": node}, bson.M{"$set": bson.M{"administrativestatus": models.USED}}); err != nil {
		return false, err
	}
	fmt.Println("updated node status")

	return true, nil
}
