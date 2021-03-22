# Index

* [Update Quobyte CSI Driver](#update-quobyte-csi-driver)
* [Update Quobyte Client](#update-quobyte-client)
  * [Application Pod Recovery](#application-pod-recovery)

## Update Quobyte CSI Driver

1. Remove existing CSI Driver

   ```bash
     helm delete <CSI-DRIVER-NAME>
   ```

2. [Install](../README.md#deploy-quobyte-csidriver) new CSI Driver

`Impact:` Removing and reinstalling Quobyte CSI driver should not
disrupt application pods with already mounted volumes. Only, new dynamic volume provisioning,
volume mount, delete requests will fail temporarily untill the new CSI Driver is
available.

## Update Quobyte Client

1. Upgrade Quobyte client on single k8s node and wait untill all application
   pods are recreated and running on that k8s node.

2. Once the application pods are running on the node with upgraded Quobyte client,
   proceed to next k8s node and repeat step 1.

`Impact:` On the particular k8s node where Quobyte client is updated, all application pods with Quobyte Volumes
will be disrupted due to the stale mount points. Application pod termination, in such case is handled by
our CSI sidecar. See [Application Pod Recovery](#application-pod-recovery) for more information.

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
