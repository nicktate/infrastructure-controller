package controller

import (
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelistersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/pkg/errors"

	"github.com/containership/cluster-manager/pkg/log"
	"github.com/containership/infrastructure-controller/pkg/etcd"
)

const (
	etcdRemovalControllerName = "EtcdRemovalController"

	etcdRemovalDelayBetweenRetriesBetweenRetries = 30 * time.Second

	maxEtcdRemovalControllerRetries = 10
)

const (
	containershipNodeIDLabelKey = "containership.io/node-id"
)

// EtcdRemovalController is a controller for removing etcd members upon a node
// being deleted from the cluster.
type EtcdRemovalController struct {
	kubeclientset kubernetes.Interface

	nodeLister  corelistersv1.NodeLister
	nodesSynced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
}

// NewEtcdRemovalController returns a new etcd removal controller
func NewEtcdRemovalController(kubeclientset kubernetes.Interface, kubeInformerFactory kubeinformers.SharedInformerFactory) *EtcdRemovalController {
	rateLimiter := workqueue.NewItemExponentialFailureRateLimiter(etcdRemovalDelayBetweenRetriesBetweenRetries, etcdRemovalDelayBetweenRetriesBetweenRetries)

	c := &EtcdRemovalController{
		kubeclientset: kubeclientset,
		workqueue:     workqueue.NewNamedRateLimitingQueue(rateLimiter, "EtcdRemoval"),
	}

	nodeInformer := kubeInformerFactory.Core().V1().Nodes()

	log.Infof("%s: Setting up event handlers", etcdRemovalControllerName)

	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.enqueueNode,
	})

	c.nodeLister = nodeInformer.Lister()
	c.nodesSynced = nodeInformer.Informer().HasSynced

	return c
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *EtcdRemovalController) Run(numWorkers int, stopCh chan struct{}) {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	log.Info(etcdRemovalControllerName, ": Starting controller")

	if ok := cache.WaitForCacheSync(stopCh, c.nodesSynced); !ok {
		// If this channel is unable to wait for caches to sync we stop both
		// all controllers
		close(stopCh)
		log.Error("failed to wait for caches to sync")
	}

	log.Info(etcdRemovalControllerName, ": Starting workers")
	// Launch numWorkers amount of workers to process resources
	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Info(etcdRemovalControllerName, ": Started workers")
	<-stopCh
	log.Info(etcdRemovalControllerName, ": Shutting down workers")
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *EtcdRemovalController) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem continually pops items off of the workqueue and handles
// them
func (c *EtcdRemovalController) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			log.Errorf("expected string in workqueue but got %#v", obj)
			return nil
		}

		err := c.syncHandler(key)
		return c.handleErr(err, key)
	}(obj)

	if err != nil {
		log.Error(err)
		return true
	}

	return true
}

// handleErr drops the key from the workqueue if the error is nil or requeues
// it up to a maximum number of times
func (c *EtcdRemovalController) handleErr(err error, key interface{}) error {
	if err == nil {
		log.Debugf("%s: Successfully synced '%s'", etcdRemovalControllerName, key)
		c.workqueue.Forget(key)
		return nil
	}

	if c.workqueue.NumRequeues(key) < maxEtcdRemovalControllerRetries {
		c.workqueue.AddRateLimited(key)
		return errors.Wrapf(err, "error syncing node %q (has been requeued %d times)", key, c.workqueue.NumRequeues(key))
	}

	c.workqueue.Forget(key)
	log.Infof("%s: Dropping node %q out of the queue: %v", etcdRemovalControllerName, key, err)
	return err
}

// enqueueNode enqueues a node.
func (c *EtcdRemovalController) enqueueNode(obj interface{}) {
	var key string
	var err error
	if key, err = nodeIDKeyFunc(obj); err != nil {
		log.Error(err)
		return
	}
	log.Debugf("%s: Adding %q to workqueue", etcdRemovalControllerName, key)
	c.workqueue.AddRateLimited(key)
}

// syncHandler requests etcd to remove the member associated with a given node
// TODO this is currently very edge-driven logic; it should really survey the
// state of the system i.e. check that the node is gone
func (c *EtcdRemovalController) syncHandler(key string) error {
	log.Infof("%s: Requesting etcd to remove member %q", etcdRemovalControllerName, key)

	client, err := etcd.NewClient(getEtcdEndpoint())
	if err != nil {
		return err
	}
	defer client.Close()

	return etcd.RemoveMember(client, key)
}

// nodeIDKeyFunc is a key function used to enqueue a node's ID instead of its name,
// since this is what we care about from etcd's perspective.
// Since only one property is used (no e.g. namespace as would be typical), no
// corresponding split function is needed.
func nodeIDKeyFunc(obj interface{}) (string, error) {
	// This is a private function intended to only be used with Node objects, so let's
	// treat it as a Node directly and avoid the meta stuff
	node, ok := obj.(*corev1.Node)
	if !ok {
		return "", errors.Errorf("cannot use node ID key function on non-Node object")
	}

	nodeID, ok := node.Labels[containershipNodeIDLabelKey]
	if !ok {
		return "", errors.Errorf("node %q does not have a label with key %q", node.Name, containershipNodeIDLabelKey)
	}

	return nodeID, nil
}

func getEtcdEndpoint() string {
	return os.Getenv("ETCD_ENDPOINT")
}
