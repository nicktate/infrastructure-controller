package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelistersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/pkg/errors"

	"github.com/containership/cluster-manager/pkg/log"
	"github.com/containership/csctl/cloud"
	provisiontypes "github.com/containership/csctl/cloud/provision/types"
	"github.com/containership/csctl/cloud/rest"
	"github.com/containership/infrastructure-controller/pkg/env"
	"github.com/containership/infrastructure-controller/pkg/etcd"
)

const (
	containershipNodeIDLabelKey = "containership.io/node-id"

	delayBetweenRequeues = 30 * time.Second

	// Don't requeue in order to avoid excessive requests to cloud for things
	// that we're going to naturally retry on the sync interval anyway
	maxRequeues = 0
)

// EtcdRemovalController is a controller for removing etcd members upon a node
// being deleted from the cluster.
type EtcdRemovalController struct {
	kubeclientset  kubernetes.Interface
	cloudclientset cloud.Interface

	nodeLister  corelistersv1.NodeLister
	nodesSynced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
}

// NewEtcdRemovalController returns a new etcd removal controller
func NewEtcdRemovalController(kubeclientset kubernetes.Interface,
	cloudclientset cloud.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory) *EtcdRemovalController {
	rateLimiter := workqueue.NewItemExponentialFailureRateLimiter(delayBetweenRequeues, maxRequeues)

	c := &EtcdRemovalController{
		kubeclientset:  kubeclientset,
		cloudclientset: cloudclientset,
		workqueue:      workqueue.NewNamedRateLimitingQueue(rateLimiter, "EtcdRemoval"),
	}

	nodeInformer := kubeInformerFactory.Core().V1().Nodes()

	log.Info("Setting up event handlers")

	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(_, obj interface{}) {
			// We don't care about compare ResourceVersions because we're
			// mainly using this handler for a periodic resync to check the
			// entire system state
			c.enqueueNode(obj)
		},
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
	log.Info("Starting controller")

	if ok := cache.WaitForCacheSync(stopCh, c.nodesSynced); !ok {
		// If this channel is unable to wait for caches to sync we stop both
		// all controllers
		close(stopCh)
		log.Error("failed to wait for caches to sync")
	}

	log.Info("Starting workers")
	// Launch numWorkers amount of workers to process resources
	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Info("Started workers")
	<-stopCh
	log.Info("Shutting down workers")
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
		c.workqueue.Forget(key)
		return nil
	}

	if c.workqueue.NumRequeues(key) < maxRequeues {
		c.workqueue.AddRateLimited(key)
		return errors.Wrapf(err, "error syncing node %q (has been requeued %d times)", key, c.workqueue.NumRequeues(key))
	}

	c.workqueue.Forget(key)
	log.Infof("Dropping node %q out of the queue: %v", key, err)
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
	c.workqueue.AddRateLimited(key)
}

// syncHandler surveys the state of the system and determines if etcd has any
// members that do not correspond to an existing node. If this is the case, it
// requests that etcd removes that member.
// The key is ignored for now because the entire state of the system is considered,
// not only the single node that was synced.
func (c *EtcdRemovalController) syncHandler(_ string) error {
	client, err := etcd.NewClient(env.EtcdEndpoint())
	if err != nil {
		return err
	}
	defer client.Close()

	nodeIDs, err := client.ListMembersByName()
	if err != nil {
		return errors.Wrap(err, "listing etcd members")
	}

	// Only remove up to one member per sync
	// No particular reason for this; just makes debugging a bit easier and we
	// periodically resync anyway
	var memberToRemove string
	for _, id := range nodeIDs {
		nodes, err := c.nodeLister.List(getContainershipNodeIDLabelSelector(id))
		if err != nil {
			return errors.Wrapf(err, "listing node with node ID %q", id)
		}

		if len(nodes) == 0 {
			log.Infof("Found etcd member named %q with no corresponding Kubernetes node", id)
			memberToRemove = id
			break
		}
	}

	if memberToRemove == "" {
		// Nothing to do
		return nil
	}

	exists, err := c.nodeExistsInCloud(memberToRemove)
	if err != nil {
		return errors.Wrapf(err, "checking if node %q exists in cloud", memberToRemove)
	}

	if exists {
		// Cloud knows about this node, so don't remove it from etcd. This can happen, for example,
		// if the master pool is scaling and the new etcd member joined but we're still waiting for
		// the new Kubernetes master node to join the cluster.
		log.Infof("Missing Kubernetes node %q exists in cloud, will skip etcd member remove", memberToRemove)
		return nil
	}

	log.Infof("Requesting etcd member remove for member with name %q", memberToRemove)
	return client.RemoveMemberByName(memberToRemove)
}

func (c *EtcdRemovalController) nodeExistsInCloud(id string) (bool, error) {
	log.Debug("Getting all etcd node pools")
	etcdPools, err := c.getNodePoolsRunningEtcd()
	if err != nil {
		return false, err
	}

	for _, np := range etcdPools {
		log.Debugf("Checking for existence of node %q in etcd node pool %q", id, string(np.ID))
		exists, err := c.nodeExistsInPool(string(np.ID), id)
		if err != nil {
			return false, err
		}

		if exists {
			return true, nil
		}
	}

	return false, nil
}

func (c *EtcdRemovalController) getNodePoolsRunningEtcd() ([]provisiontypes.NodePool, error) {
	nodePools, err := c.cloudclientset.Provision().NodePools(env.OrganizationID(), env.ClusterID()).List()
	if err != nil {
		return nil, errors.Wrap(err, "listing node pools")
	}

	etcdPools := make([]provisiontypes.NodePool, 0)
	for _, np := range nodePools {
		if np.Etcd != nil && *np.Etcd {
			etcdPools = append(etcdPools, np)
		}
	}

	return etcdPools, nil
}

func (c *EtcdRemovalController) nodeExistsInPool(nodePoolID, nodeID string) (bool, error) {
	_, err := c.cloudclientset.Provision().Nodes(env.OrganizationID(), env.ClusterID(), nodePoolID).Get(nodeID)
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

func getContainershipNodeIDLabelSelector(id string) labels.Selector {
	selector := labels.NewSelector()
	req, _ := labels.NewRequirement(containershipNodeIDLabelKey, selection.Equals, []string{id})
	return selector.Add(*req)
}
