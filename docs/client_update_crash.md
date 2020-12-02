# Index

* [Quobyte Client Upgrade Procedure](#quobyte-client-upgrade-procedure)
* [Application Pod Recovery](#application-pod-recovery)

## Quobyte Client Upgrade Procedure

1. Upgrade Quobyte client on single k8s node and wait untill all application
   pods are recreated and running on that k8s node.

2. Once the application pods are running on node with upgraded Quobyte client,
   proceed to next k8s node and repeat step 1.

## Application Pod Recovery

A crashing Quobyte client/upgraded Quobyte client leaves all the existing application
pods with Quobyte CSI volumes in invalid and stale state on that particular k8s node
where Quobyte client is upgraded or crashed. To recover, existing application pod must
be deleted and a new pod should be created. The pod monitoring sidecar container shipped
with Quobyte CSI automatically deletes the application pods with stale Quobyte CSI volumes
and leaves the creation of new application pod to k8s.

For k8s to schedule a new pod in place of the killed pod with stale Quobyte mounts,
your application should be deployed as `Deployment/Statefulsets/Replicaset` but
not as a plain `Pod`.
