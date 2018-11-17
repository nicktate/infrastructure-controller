package etcd

import (
	"context"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/pkg/transport"
	"github.com/pkg/errors"

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
func NewClient(endpoint string) (*Client, error) {
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
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
		TLS:         tlsConfig,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "etcd client connect to endpoint %q failed", endpoint)
	}

	return &Client{client}, nil
}

// ListMembersByName returns the names of all etcd members or an error.
func (c *Client) ListMembersByName() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.MemberList(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "etcd client failed to list members")
	}

	names := make([]string, len(resp.Members))
	for i, m := range resp.Members {
		names[i] = m.Name
	}

	return names, nil
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
