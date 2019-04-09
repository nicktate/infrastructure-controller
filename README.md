[![CircleCI](https://circleci.com/gh/containership/infrastructure-controller.svg?style=svg)](https://circleci.com/gh/containership/infrastructure-controller)
[![Go Report Card](https://goreportcard.com/badge/github.com/containership/infrastructure-controller)](https://goreportcard.com/report/github.com/containership/infrastructure-controller)
[![codecov](https://codecov.io/gh/containership/infrastructure-controller/branch/master/graph/badge.svg)](https://codecov.io/gh/containership/infrastructure-controller)

[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v1.4%20adopted-ff69b4.svg)](code-of-conduct.md)

# Containership Cloud Infrastructure Controller

The infrastructure controller is a simple controller for augmenting the functionality of Containership Kubernetes Engine (CKE) clusters in ways that are specific to infrastructure management.
In other words, it splits off from the core Containership [cluster-manager][cluster-manager] anything infrastructure-specific.

Currently, the only feature this component has is removing stale etcd members.

## Etcd Member Removal

CKE clusters run etcd via [static pods][static-pods].
If a Kubernetes node is deleted for any reason (through a master pool scale down, for example), then nothing will automatically remove the corresponding etcd member from the etcd cluster.
For this reason, we built the etcd removal controller to reconcile Kubernetes nodes and their associated etcd members.

### How it Works

The etcd member removal controller simply has an informer on Kubernetes nodes.
If the number of nodes that should be running etcd does not match the actual etcd member list, then action is taken to reconcile this difference.

Containership Cloud is used as the source of truth for which Kubernetes nodes should exist as a precaution before taking any actions.
This is to prevent mishandling edge cases such as: a new master node is being added, so the etcd member has been added but the corresponding node does not exist in Kubernetes yet.

### Notes

This controller does not enforce any rules around proper scaling actions.
Containership Cloud ensures that a user cannot scale down an etcd cluster by more than a certain amount at a time.

[Cluster-manager]: https://github.com/containership/cluster-manager
[static-pods]: https://kubernetes.io/docs/tasks/administer-cluster/static-pod/
