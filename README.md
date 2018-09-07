# Quobyte CSI

Quobyte CSI is the implementation of
 [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec/tree/v0.2.0).
 Quobyte CSI enables easy integration of Quobyte Storage into Kubernetes. Current Quobyte CSI plugin
 supports the following functionality

* Dynamic volume Create
* Volume Delete
* Pre-provisioned volumes (Delete policy does not apply to these volumes)

## Requirements

* Kubernetes v1.10.7 or higher
* Quobyte installation with reachable registry and api services from the nodes
* Quobyte client with
  * `QUOBYTE_REGISTRY` environment variable set with Quobyte registry
  * `QUOBYTE_MOUNT_POINT` environment variable set to `/mnt/quobyte/mounts`
  * host path volume `/mnt/quobyte`
* Additionally, Kubernetes CSI requires some Kubernetes helper containers and corresponding RBAC
 permissions

Deploy RBAC as required by [CSI](https://kubernetes-csi.github.io/docs/Example.html) and CSI helper
 containers along with Quobyte CSI plugin containers

```bash
kubectl create -f attacher-rbac.yaml
kubectl create -f nodeplugin-rbac.yaml
kubectl create -f provisioner-rbac.yaml
kubectl create -f attacher.yaml
kubectl create -f plugin.yaml
kubectl create -f provisioner.yaml
```

Quobyte requires a secret to authenticate volume create and delete requests. Create this secret with
 your Quobyte login credentials. Kubernetes requires base64 encoding for secrets which can be created
 with the command `echo -n "<user>" | base64`.

```bash
kubectl create -f example/csi-secret.yaml
```

Create a storage class with the `provisioner` set to `quobyte-csi` along with other configuration
 parameters.

```bash
kubectl create -f example/StorageClass.yaml
```

## Dynamic volume provisioning

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

## Using existing volumes

Quobyte CSI requires the volume UUID to be passed on to the PV as `VolumeHandle`  

In order to use the `test` volume belonging to the tenant `My Test`, user needs to create a PV
 referring the volume as shown in the `example/pv-existing-vol.yaml`  
`NOTE:`

* Quobyte-csi supports both volume name and UUID
  * **To use Volume Name** `VolumeHandle` should be of the format `<API_URL>|<Tenant_Name/UUID>|<Volume_Name>`
   and `nodePublishSecretRef` with Quobyte API login credentials should be specified as shown in the
   example PV `example/pv-existing-vol.yaml`
  * **To use Volume UUID** `VolumeHandle` can be `||<Volume_UUID>`.

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