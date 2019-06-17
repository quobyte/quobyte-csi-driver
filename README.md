# Quobyte CSI

Quobyte CSI is the implementation of
 [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec/tree/v0.2.0).
 Quobyte CSI enables easy integration of Quobyte Storage into Kubernetes. Current Quobyte CSI plugin
 supports the following functionality

* Dynamic volume Create
* Volume Delete
* Pre-provisioned volumes (Delete policy does not apply to these volumes)

## Requirements

* Kubernetes v1.13 or higher
  * On K8S v1.13, `CSIDriverRegistry` feature gate must be enabled.
* Quobyte installation with reachable registry and api services from the nodes
* Quobyte client with (please see `example/client.yaml` for sample configuration)
  * `QUOBYTE_REGISTRY` environment variable set with Quobyte registry
  * `QUOBYTE_MOUNT_POINT` environment variable set to `/mnt/quobyte/mounts`
  * host path volume `/mnt/quobyte`  
  Alternatively, Kubernetes nodes can have Quobyte native client with mount path as `/mnt/quobyte/mounts`

## Deploy Quobyte CSI

1. Edit `deploy/config.yaml` and configure `quobyte.apiURL` with your Quobyte cluster API URL
2. Create configuration

```kubectl create -f deploy/config.yaml```

3. Deploy RBAC as required by [CSI](https://kubernetes-csi.github.io/docs/Example.html) and CSI helper
 containers along with Quobyte CSI plugin containers

```bash
kubectl create -f deploy/deploy-csi-driver-1.0.1.yaml
```

4. Verify the status of Quobyte CSI driver pods

```
kubectl -n kube-system get po -owide | grep ^quobyte-csi
```

The Quobyte CSI plugin is ready to use, if you see `quobyte-csi-controller-x` pod running on any one node and `quobyte-csi-node-xxxxx`
 running on every node of the Kubernetes cluster.

## Use Quobyte volumes in Kubernetes

Quobyte requires a secret to authenticate volume create and delete requests. Create this secret with
 your Quobyte login credentials. Kubernetes requires base64 encoding for secrets which can be created
 with the command `echo -n "<user>" | base64`.

```bash
kubectl create -f example/csi-secret.yaml
```

Create a storage class with the `provisioner` set to `csi.quobyte.com` along with other configuration
 parameters.

```bash
kubectl create -f example/StorageClass.yaml
```

### Dynamic volume provisioning

Creating a PVC referencing the storage class created in previous step would provision dynamic
 volume.The provisoning happens through Kubernetes CSI - creates the PV inside Kubernetes and
 Quobyte CSI- provisions the volume for created PV.

```bash
kubectl create -f example/pvc-dynamic-provision.yaml
```

Mount the PVC in a pod as shown in the following example

```bash
kubectl create -f example/pod-with-dynamic-vol.yaml
```

### Using existing volumes

Quobyte CSI requires the volume UUID to be passed on to the PV as `VolumeHandle`  

In order to use the `test` volume belonging to the tenant `My Test`, user needs to create a PV
 referring the volume as shown in the `example/pv-existing-vol.yaml`  
`NOTE:`

* Quobyte-csi supports both volume name and UUID
  * **To use Volume Name** `VolumeHandle` should be of the format `<Tenant_Name/UUID>|<Volume_Name>`
   and `nodePublishSecretRef` with Quobyte API login credentials should be specified as shown in the
   example PV `example/pv-existing-vol.yaml`
  * **To use Volume UUID** `VolumeHandle` can be `|<Volume_UUID>`.

```bash
kubectl create -f example/pv-existing-vol.yaml
```

Create a PVC that matches the storage requirements with the above PV (make sure both PV and PVC refer
 to the same storage class)

```bash
kubectl create -f example/pvc-existing-vol.yaml
```

Create a pod referring the PVC as shown in the below example

```bash
kubectl create -f example/pod-with-existing-vol.yaml
```
