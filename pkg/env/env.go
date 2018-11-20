package env

import (
	"io"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"

	"github.com/containership/cluster-manager/pkg/log"
)

type environment struct {
	// Required
	organizationID string
	clusterID      string
	clusterToken   string
	etcdEndpoint   string

	// Optional
	csCloudEnvironment string
	provisionBaseURL   string
}

var env environment

// Init initializes the environment, returning an error if any required values
// are not provided
func Init() error {
	env.csCloudEnvironment = strings.ToLower(os.Getenv("CONTAINERSHIP_CLOUD_ENVIRONMENT"))
	if env.csCloudEnvironment == "" {
		env.csCloudEnvironment = "production"
	}

	env.organizationID = os.Getenv("CONTAINERSHIP_CLOUD_ORGANIZATION_ID")
	if env.organizationID == "" {
		return errors.New("CONTAINERSHIP_CLOUD_ORGANIZATION_ID env var must be specified")
	}

	env.clusterID = os.Getenv("CONTAINERSHIP_CLOUD_CLUSTER_ID")
	if env.clusterID == "" {
		return errors.New("CONTAINERSHIP_CLOUD_CLUSTER_ID env var must be specified")
	}

	env.clusterToken = os.Getenv("CONTAINERSHIP_CLOUD_CLUSTER_API_KEY")
	if env.clusterToken == "" {
		return errors.New("CONTAINERSHIP_CLOUD_CLUSTER_API_KEY env var must be specified")
	}

	env.etcdEndpoint = os.Getenv("ETCD_ENDPOINT")
	if env.etcdEndpoint == "" {
		return errors.New("ETCD_ENDPOINT env var must be specified")
	}

	// Client will default if needed
	env.provisionBaseURL = os.Getenv("CONTAINERSHIP_CLOUD_PROVISION_BASE_URL")

	return nil
}

// OrganizationID returns the organization ID
func OrganizationID() string {
	return env.organizationID
}

// ClusterID returns the cluster ID we're running in
func ClusterID() string {
	return env.clusterID
}

// EtcdEndpoint returns the etcd endpoint
func EtcdEndpoint() string {
	return env.etcdEndpoint
}

// ClusterToken returns the
func ClusterToken() string {
	return env.clusterToken
}

// ProvisionBaseURL returns the provision service base URL
func ProvisionBaseURL() string {
	return env.provisionBaseURL
}

// Dump dumps the environment to the given io.Writer
// if we're in a development or stage environment
func Dump(w io.Writer) {
	if env.csCloudEnvironment == "development" || env.csCloudEnvironment == "stage" {
		dump := spew.Sdump(env)
		log.Debug(dump)
	}
}
