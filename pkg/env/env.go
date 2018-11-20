package env

import (
	"os"
)

func OrganizationID() string {
	return os.Getenv("CONTAINERSHIP_CLOUD_ORGANIZATION_ID")
}

func ClusterID() string {
	return os.Getenv("CONTAINERSHIP_CLOUD_CLUSTER_ID")
}

func EtcdEndpoint() string {
	return os.Getenv("ETCD_ENDPOINT")
}

func ClusterToken() string {
	return os.Getenv("CONTAINERSHIP_CLOUD_CLUSTER_API_KEY")
}

func ProvisionBaseURL() string {
	return os.Getenv("CONTAINERSHIP_CLOUD_PROVISION_BASE_URL")
}
