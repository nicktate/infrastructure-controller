package etcd

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/coreos/etcd/pkg/transport"

	"github.com/containership/cluster-manager/pkg/log"
)

const (
	clientCertPath = "/etcd/client.crt"
	clientKeyPath  = "/etcd/client.key"
	caCertPath     = "/etcd/ca.crt"
)

// Client is a simplified etcd client
type Client struct {
	*clientv3.Client
}

// NewClient returns a new etcd client for the given endpoint, using the
// well-known certificate and key files from disk. If the error returned is
// nil, then the client is already connected (this is due to how the etcd
// clientv3 API works).  The caller is responsible for calling client.Close().
func NewClient(endpoints []string) (*Client, error) {
	tlsInfo := transport.TLSInfo{
		CertFile:      clientCertPath,
		KeyFile:       clientKeyPath,
		TrustedCAFile: caCertPath,
	}
	tlsConfig, err := tlsInfo.ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error creating etcd TLS client config")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
		TLS:         tlsConfig,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "etcd client connect to endpoints %v failed", endpoints)
	}

	return &Client{client}, nil
}

// ListMembers returns all etcd members or an error.
func (c *Client) ListMembers() ([]*etcdserverpb.Member, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.MemberList(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "etcd client failed to list members")
	}

	return resp.Members, nil
}

// RemoveMemberByName removes the etcd member with the given name.
func (c *Client) RemoveMemberByName(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.MemberList(ctx)
	if err != nil {
		return errors.Wrap(err, "etcd client failed to list members")
	}

	// Find the member ID of the member with the provided name
	var memberID uint64
	for _, m := range resp.Members {
		if m.Name == name {
			memberID = m.ID
			break
		}
	}

	if memberID == 0 {
		return errors.Errorf("cannot remove nonexistent etcd member %q", name)
	}

	_, err = c.MemberRemove(ctx, memberID)
	if err != nil {
		return errors.Wrapf(err, "etcd client failed to remove member %q", name)
	}

	log.Infof("etcd member %q (ID %x) removed successfully", name, memberID)
	return nil
}

// MemberIsHealthy returns true if the given member is healthy, else false.
func MemberIsHealthy(member *etcdserverpb.Member) bool {
	healthcheckClient, err := NewClient(member.PeerURLs)
	if err != nil {
		// Because constructing an etcd client also connects to it, if constructing
		// it fails then we'll just call the member unhealthy and short-circuit
		// instead of bubbling up errors.
		log.Debugf("etcd member %q assumed unhealthy because we can't connect (error: %s)", member.Name, err)
		return false
	}
	defer healthcheckClient.Close()

	// The below mimics how the `etcdctl endpoint health` command works under the hood.
	// Simply get a random key and if no error occurs, the member is assumed to be healthy.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err = healthcheckClient.Get(ctx, "health")
	defer cancel()

	// Permission denied is ok because it requires consensus - see etcdctl source.
	return err == nil || err == rpctypes.ErrPermissionDenied
}
