package main

import (
	"flag"
	"os"
	"runtime"
	"time"

	"github.com/pkg/errors"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/containership/cluster-manager/pkg/log"
	"github.com/containership/infrastructure-controller/pkg/buildinfo"
	"github.com/containership/infrastructure-controller/pkg/controller"
)

func main() {
	log.Info("Starting Containership Infrastructure Controller...")
	log.Infof("Version: %s", buildinfo.String())
	log.Infof("Go Version: %s", runtime.Version())

	// We don't have any of our own flags to parse, but k8s packages want to
	// use glog and we have to pass flags to that to configure it to behave
	// in a sane way.
	flag.Parse()

	config, err := determineConfig()
	if err != nil {
		log.Fatal(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clientset: %+v", err)
	}

	kubeInformerFactory := informers.NewSharedInformerFactory(clientset, 15*time.Second)

	etcdRemovalController := controller.NewEtcdRemovalController(
		clientset, kubeInformerFactory)

	stopCh := make(chan struct{})
	kubeInformerFactory.Start(stopCh)

	go etcdRemovalController.Run(1, stopCh)

	runtime.Goexit()
}

// determineConfig determines if we are running in a cluster or outside
// and gets the appropriate configuration to talk with Kubernetes.
func determineConfig() (*rest.Config, error) {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	var config *rest.Config
	var err error

	// determine whether to use in cluster config or out of cluster config
	// if kubeconfigPath is not specified, default to in cluster config
	// otherwise, use out of cluster config
	if kubeconfigPath == "" {
		log.Info("Using in cluster k8s config")
		config, err = rest.InClusterConfig()
	} else {
		log.Info("Using out of cluster k8s config: ", kubeconfigPath)

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	if err != nil {
		return nil, errors.Wrap(err, "determine Kubernetes config failed")
	}

	return config, nil
}
