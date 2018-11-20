package node

import (
	"github.com/pkg/errors"

	"github.com/containership/cluster-manager/pkg/log"
	"github.com/containership/csctl/cloud"
	"github.com/containership/csctl/cloud/provision/types"
	"github.com/containership/csctl/cloud/rest"

	"github.com/containership/infrastructure-controller/pkg/env"
)

// ExistsInCloud checks if the node with the given ID exists in Containership cloud
// It checks only pools that are running etcd, because it's intended that callers
// only care about etcd nodes. Because a node pool object from cloud does not
// contain the actual node pool IDs, additional queries to the /nodes route are required.
func ExistsInCloud(clientset cloud.Interface, id string) (bool, error) {
	log.Debug("Getting all etcd node pools")
	etcdPools, err := getNodePoolsRunningEtcd(clientset)
	if err != nil {
		return false, err
	}

	for _, np := range etcdPools {
		log.Debugf("Checking for existence of node %q in etcd node pool %q", id, string(np.ID))
		exists, err := nodeExistsInPool(clientset, string(np.ID), id)
		if err != nil {
			return false, err
		}

		if exists {
			return true, nil
		}
	}

	return false, nil
}

// getNodePoolsRunningEtcd gets all node pools that are running etcd from Containership
func getNodePoolsRunningEtcd(clientset cloud.Interface) ([]types.NodePool, error) {
	nodePools, err := clientset.Provision().NodePools(env.OrganizationID(), env.ClusterID()).List()
	if err != nil {
		return nil, errors.Wrap(err, "listing node pools")
	}

	etcdPools := make([]types.NodePool, 0)
	for _, np := range nodePools {
		if np.Etcd != nil && *np.Etcd {
			etcdPools = append(etcdPools, np)
		}
	}

	return etcdPools, nil
}

// nodeExistsInPool returns true if the node with the given ID exists in the
// given node pool, else false.
func nodeExistsInPool(clientset cloud.Interface, nodePoolID, nodeID string) (bool, error) {
	_, err := clientset.Provision().Nodes(env.OrganizationID(), env.ClusterID(), nodePoolID).Get(nodeID)
	if err == nil {
		// Found it
		return true, nil
	}

	switch err := err.(type) {
	case rest.HTTPError:
		if err.IsNotFound() {
			return false, nil
		}
	}

	// Some other error occurred
	return false, errors.Wrapf(err, "attempting to get node %q from pool %q", nodeID, nodePoolID)
}
