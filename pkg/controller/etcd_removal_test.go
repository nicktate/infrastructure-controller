package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coreos/etcd/etcdserver/etcdserverpb"

	"github.com/containership/csctl/cloud/provision/types"
)

var (
	emptyNode = types.Node{}

	node1 = types.Node{
		Addresses: &types.NodeAddresses{
			InternalIP: "1.1.1.1",
		},
	}

	node2 = types.Node{
		Addresses: &types.NodeAddresses{
			InternalIP: "2.2.2.2",
		},
	}

	node3 = types.Node{
		Addresses: &types.NodeAddresses{
			InternalIP: "3.3.3.3",
		},
	}
)

func TestEtcdMemberHasCloudNodeCounterpart(t *testing.T) {
	result := etcdMemberHasCloudNodeCounterpart(nil, []types.Node{node1})
	assert.False(t, result, "nil member always returns false")

	member := &etcdserverpb.Member{
		PeerURLs: []string{"https://1.1.1.1:2379"},
	}
	result = etcdMemberHasCloudNodeCounterpart(member, []types.Node{})
	assert.False(t, result, "empty nodes always returns false")

	result = etcdMemberHasCloudNodeCounterpart(member, []types.Node{emptyNode})
	assert.False(t, result, "node missing internal IP always returns false")

	result = etcdMemberHasCloudNodeCounterpart(member, []types.Node{node1})
	assert.True(t, result, "matching node in pool of one")

	result = etcdMemberHasCloudNodeCounterpart(member, []types.Node{node1, node2, node3})
	assert.True(t, result, "matching node in pool of multiple")

	member = &etcdserverpb.Member{
		PeerURLs: []string{"https://9.9.9.9:2379"},
	}
	result = etcdMemberHasCloudNodeCounterpart(member, []types.Node{node1, node2, node3})
	assert.False(t, result, "no matching nodes")
}
