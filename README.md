# Quobyte CSI

Quobyte CSI is the implementation of
 [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec/tree/release-1.0).
 Quobyte CSI enables easy integration of Quobyte Storage into Kubernetes. Current Quobyte CSI driver
 supports the following functionality

* Dynamic Volume Create
* Volume Delete
* Pre-provisioned volumes (Delete policy does not apply to these volumes)
* Volume Expansion (Only dynamically provisioned volumes can be expanded)
  * Quobyte supports volumes with unlimited size, expanding an unlimited sized
    volume restricts the volume size to expanded size.
* Volume snapshots

## Select Quobyte CSI Driver Release

1. Choose a Quobyte CSI release from [available releases](https://github.com/quobyte/quobyte-csi/releases)

2. Follow the instructions specific to that release

## Index

* [Requirements](#requirements)
* [Deploy Quobyte clients](docs/install_client)
  * [Quobyte 2.x](docs/install_client/deploy_clients_2_x.md)
  * [Quobyte 3.x](docs/install_client/deploy_clients_3_x.md)
* [Deploy Quobyte CSI](#deploy-quobyte-csi-driver)
* [Quobyte Version Compatibility](docs/quobyte_versions.md)
* [Snapshotter Setup](#snapshotter-setup) (**required only if snapshots are enabled**)
* [Usage Examples](#use-quobyte-volumes-in-kubernetes)
  * [Use Quobyte volumes in Kubernetes](#use-quobyte-volumes-in-kubernetes)
    * [Dynamic volume provisioning](#dynamic-volume-provisioning)
    * [Use existing volumes](#use-existing-volumes)
  * [Volume Snapshots](#volume-snapshots)
    * [Dynamic Snapshots](#dynamic-snapshots)
    * [Pre-provisioned Snapshots](#pre\-provisioned-snapshots)
  * [Shared Volumes](usage-example/shared-volume/README.md)
* [Update Quobyte CSI or Client](docs/update_quobyte_csi_or_clients.md#index)
  * [Update Quobyte CSI Driver](docs/update_quobyte_csi_or_clients.md#update-quobyte-csi-driver)
  * [Update Quobyte Client](docs/update_quobyte_csi_or_clients.md#update-quobyte-client)
    * [Application pod recovery](docs/update_quobyte_csi_or_clients.md#application-pod-recovery)
* [Harden Access with Quobyte Access Keys](docs/quobyte_access_keys.md) (**requires Quobyte 3.1 or later**)
* [Uninstall Quobyte CSI](#uninstall-quobyte-csi)
* [Quobyte Client Upgrade Example](docs/client_update_example.md)
* [Multi-cluster setup](docs/multi-cluster-setup.md)
* [Collect Quobyte CSI logs](docs/collect_quobyte_csi_logs.md)
* [Run e2e tests on kind cluster](docs/Kind-cluster-for-e2e-tests.md)

## Requirements

* Requires [Helm](https://helm.sh/docs/intro/install/#from-script) and [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/)
* Requires at least Kubernetes v1.22
* Quobyte installation with reachable registry and api services from the Kubernetes nodes and pods
* Quobyte client with mount path as
  <[values.clientMountPoint](https://github.com/quobyte/quobyte-csi/blob/4671450b0dec5fe162f78f9e35c6c6fe90e3f86b/quobyte-csi-driver/values.yaml#L18)>`/mounts`. Please see [Deploy Quobyte clients](docs/install_client) for Quobyte client installation instructions.
  * To use Quobyte access keys, the Quobyte client (requires Quobyte version 3.0 or above) should
   be deployed with **--enable-access-contexts**. Additionally, the metadata cache (global policy)
   should be disabled.
* If you have load balancer for Quobyte API, the load balancer must be configured with sticky
  sessions.
* Requires [additional setup](#snapshotter-setup) to use volume snapshots

## Deploy Quobyte CSI Driver

**Note:** Quobyte CSI driver automatically deletes all the application pods with
 [stale](https://github.com/kubernetes/kubernetes/issues/70013) Quobyte CSI volumes
 and leaves the new pod creation to kubernetes. To reschedule a new pod
 automatically by k8s, applications should be deployed with `Deployment/ReplicaSet/StatefulSets`
 but not as a plain `Pod`.

1. Add `quobyte-csi-driver` helm repository to your `helm` repos

    ```bash
    helm repo add quobyte-csi-driver https://quobyte.github.io/quobyte-csi-driver/helm
    ```

    If the `quobyte-csi-driver` helm repo already exists in your helm repositories, you should update
    the repo to get the new Quobyte CSI Driver releases. Update the repo

    ```bash
    helm repo update quobyte-csi-driver
    ``` 

2. List all available Quobyte CSI versions

    ```bash
    helm search repo quobyte-csi-driver/quobyte-csi-driver -l
    ```

3. List all customization options for Quobyte CSI driver

    ```bash
    helm show values quobyte-csi/quobyte-csi [--version <chart-version>] # or use other "show <subcommands>"
    ```

4. Edit [Quobyte CSI driver configuration](quobyte-csi-driver/values.yaml) (./quobyte-csi-driver/values.yaml) and configure CSI driver
   with Quobyte API, other required information.

5. (optional) generate driver deployment `.yaml` and verify the configuration.

    ```bash
    helm template ./quobyte-csi-driver --debug > csi-driver.yaml
    ```

6. Deploy the Quobyte CSI driver with customizations

    ```bash
    # Deploys helm chart with name "quobyte-csi".
    # Please change quobyte-csi as required
    helm install quobyte-csi quobyte-csi-driver/quobyte-csi-driver [--version <chart-version>]
      \ --set quobyte.apiURL="<your-api-url>" ....
    ```

    or

    ```bash
    helm install quobyte-csi quobyte-csi-driver/quobyte-csi-driver [--version <chart-version>]
      \ -f <your-customized-values.yaml> [--set quobyte.apiURL="<your-api-url>" .. other overrides]
    ```

7. Verify the status of Quobyte CSI driver pods

    Deploying Quobyte CSI driver should create a CSIDriver object
     with your `csiProvisionerName` (this may take few seconds)

    ```bash
    CSI_PROVISIONER="<YOUR-csiProvisionerName>"
    kubectl get CSIDriver | grep ^${CSI_PROVISIONER}
    ```

    The Quobyte CSI driver is ready for use, if you see `quobyte-csi-controller-x`
    pod running on any one node and `quobyte-csi-node-xxxxx`
    running on every node of the Kubernetes cluster.

    ```bash
    CSI_PROVISIONER=$(echo $CSI_PROVISIONER | tr "." "-")
    kubectl -n kube-system get po -owide | grep ^quobyte-csi-.*-${CSI_PROVISIONER}
    ```

8. Make sure your CSI driver is running against the expected Quobyte API endpoint

    ```bash
    kubectl -n kube-system exec -it \
    "$(kubectl get po -n kube-system | grep -m 1 ^quobyte-csi-node-$CSI_PROVISIONER \
    |  cut -f 1 -d' ')" -c quobyte-csi-driver -- env | grep QUOBYTE_API_URL
    ```

    The above command should print your Quobyte API endpoint. Otherwise, uninstall
    Quobyte CSI driver and install again with the correct Quobyte API URL.

## Examples

`Note:` [k8s storage class](https://kubernetes.io/docs/concepts/storage/storage-classes/) is
  immutable. Do not delete existing definitions, such a deletion could cause issues for existing
  PV/PVCs.

### Use Quobyte volumes in Kubernetes

`Note:` This section uses `example/` deployment files for demonstration. These should be modified
  with your deployment configurations such as `namespace`, `quobyte registry`, `Quobyte API user credentials` etc.

We use `quobyte` namespace for the examples. Create the namespace

  ```bash
  kubectl create ns quobyte
  ```

Quobyte requires a secret to authenticate volume create and delete requests. Create this secret with
 your Quobyte API login credentials (Kubernetes requires base64 encoding for secret data which can be obtained
 with the command `echo -n "value" | base64`). Please encode your user name, password (and optionally access key
 information) in base64 and update [example/quobyte-admin-credentials.yaml](example/quobyte-admin-credentials.yaml). If provided, access key
 ensures only authorized user can access the tenant and volumes (users must be restricted to their own namespace in k8s cluster).

  ```bash
  kubectl create -f example/quobyte-admin-credentials.yaml
  ```

Create a [storage class](example/StorageClass.yaml) with the `provisioner` set to `csi.quobyte.com` along with other configuration
 parameters. You could create multiple storage classes by varying `parameters` such as
  `quobyteTenant`, `quobyteConfig` etc.

  ```bash
  kubectl create -f example/StorageClass.yaml
  ```

#### Dynamic volume provisioning

Creating a PVC referencing the storage class created in the previous step would provision dynamic
 volume. The secret `csi.storage.k8s.io/provisioner-secret-name` from the namespace `csi.storage.k8s.io/provisioner-secret-namespace`
 in the referenced StorageClass will be used to authenticate volume creation and deletion.

1. Create [PVC](example/pvc-dynamic-provision.yaml) to trigger dynamic provisioning

    ```bash
    kubectl create -f example/pvc-dynamic-provision.yaml
    ```

2. Mount the PVC in a [pod](example/nginx-demo-pod-with-dynamic-vol.yaml) as shown in the following example

    ```bash
    kubectl create -f example/nginx-demo-pod-with-dynamic-vol.yaml
    ```

3. Wait for the pod to be in running state

    ```bash
    kubectl get po -w | grep 'nginx-dynamic-vol'
    ```

4. Once the pod is running, copy the [index file](example/index.html) to the deployed nginx pod

    ```bash
    kubectl cp example/index.html nginx-dynamic-vol:/tmp
    kubectl exec -it nginx-dynamic-vol -- mv /tmp/index.html /usr/share/nginx/html/
    kubectl exec -it nginx-dynamic-vol -- chown -R nginx:nginx /usr/share/nginx/html/
    ```

5. Access the home page served by nginx pod from the command line

    ```bash
    curl http://$(kubectl get pods nginx-dynamic-vol -o yaml | grep ' podIP:' | awk '{print $2}'):80
    ```

    Above command should retrieve the Quobyte CSI welcome page (in raw html format). If encountered
    error, see if you need to forward your local port to pod.

    NOTE: Depending on your cluster setup (for example, kind clusters), you may need to forward your
    local port to container to access the nginx pod port. In such case, you could use

    ```bash
    kubectl port-forward nginx-dynamic-vol 8086:80
    ```

  and then try `curl localhost:8086`

#### Use existing volumes

Quobyte CSI requires the volume UUID to be passed on to the PV as `VolumeHandle`  

* Quobyte-csi supports both volume name and UUID
  * **To use Volume Name** `VolumeHandle` should be of the format `<Tenant_Name/UUID>|<Volume_Name>`
   and `nodePublishSecretRef` with Quobyte API login credentials should be specified as shown in the
   example PV `example/pv-existing-vol.yaml`
  * **To use Volume UUID** `VolumeHandle` can be `|<Volume_UUID>`.

In order to use the pre-provisioned `test` volume belonging to the tenant `My Tenant`, user needs to create
 a PV with `volumeHandle: My Tenant|test` as shown in the [example PV](example/pv-existing-vol.yaml).

1. Edit [example/pv-existing-vol.yaml](example/pv-existing-vol.yaml) and point it to the the pre-provisioned volume in Quobyte
 storage through `volumeHandle`. Create the PV with pre-provisioned volume.

    ```bash
    kubectl create -f example/pv-existing-vol.yaml
    ```

2. Create a [PVC](example/pvc-existing-vol.yaml) that matches the storage requirements with the above PV (make sure both PV and PVC refer
 to the same storage class). The created PVC will automatically binds to the PV.

    ```bash
    kubectl create -f example/pvc-existing-vol.yaml
    ```

3. Create a [pod](example/nginx-demo-pod-with-existing-vol.yaml) referring the PVC as shown in the below example

    ```bash
    kubectl create -f example/nginx-demo-pod-with-existing-vol.yaml
    ```

4. Wait for the pod to be in running state

    ```bash
    kubectl get po -w | grep 'nginx-existing-vol'
    ```

5. Once the pod is running, copy the [index file](example/index.html) to the deployed nginx pod

    ```bash
    kubectl cp example/index.html nginx-existing-vol:/tmp
    kubectl exec -it nginx-existing-vol -- mv /tmp/index.html /usr/share/nginx/html/
    kubectl exec -it nginx-existing-vol -- chown -R nginx:nginx /usr/share/nginx/html/
    ```

6. Access the home page served by nginx pod from the command line

    ```bash
    curl http://$(kubectl get pods nginx-existing-vol -o yaml | grep ' podIP:' | awk '{print $2}'):80
    ```

    Above command should retrieve the Quobyte CSI welcome page (in raw html format). If encountered
    error, see if you need to forward your local port to pod.

    NOTE: Depending on your cluster setup (for example, kind clusters), you may need to forward your
    local port to container to access the nginx pod port. In such case, you could use

    ```bash
    kubectl port-forward nginx-dynamic-vol 8086:80
    ```

  and then try `curl localhost:8086`

### Volume snapshots

#### Snapshot Requirements

1. [Quobyte CSI Driver](./quobyte-csi-driver/values.yaml) is deployed with `enableSnapshots: true`

2. [Snapshotter setup](#snapshotter-setup)

##### Dynamic Snapshots

  1. Provision a PVC for a Quobyte volume by following the [instructions](#use-quobyte-volumes-in-kubernetes)

  2. Populate backing volume with [nginx index file](example/index.html)

      ```bash
      VOLUME="<Quobyte-Volume>" # volume for which snapshot will be taken
      wget https://raw.githubusercontent.com/quobyte/quobyte-csi/master/example/index.html -P <values.clientMountPoint>/mounts/$VOLUME
      ```

  3. Create [volume snapshot secrets](example/quobyte-admin-credentials.yaml)

     Our examples use same secret in all the places wherever secret is required. Please create and
     configure secrets as per your requirements.

        ```bash
        kubectl create -f example/quobyte-admin-credentials.yaml
        ```

  4. Create volume [snapshot class](example/volume-snapshot-class.yaml)

        ```bash
        kubectl create -f example/volume-snapshot-class.yaml
        ```

  5. Create [dynamic volume snapshot](example/volume-snapshot-dynamic-provision.yaml)

        ```bash
        kubectl create -f example/volume-snapshot-dynamic-provision.yaml
        ```

     The above command should create required `volumesnapshotcontent` object dynamically

  6. (optional) verify created `volumesnapshot` and `volumesnapshotcontent` objects

        ```bash
        kubectl get volumesnapshot
        kubectl get volumesnapshotcontent
        ```

  7. [Restore snapshot](example/restore-snapshot-pvc-dynamic-provision.yaml) and create PVC

        ```bash
        kubectl create -f example/restore-snapshot-pvc-dynamic-provision.yaml
        ```

     This should create a PVC and a PV for the restored snapshot

  8. Create pod with [restored snapshot](example/nginx-demo-pod-with-dynamic-snapshot-vol.yaml)

        ```bash
        kubectl create -f example/nginx-demo-pod-with-dynamic-snapshot-vol.yaml
        ```

##### Pre-provisioned Snapshots

  1. Create volume [snapshot class](example/volume-snapshot-class.yaml)

        ```bash
        kubectl create -f example/volume-snapshot-class.yaml
        ```

  2. Create volume snapshot secrets

     Our examples use same secret in all the places wherever secret is required.
      Please create and configure secrets as per your requirements.

      ```bash
      kubectl create -f example/quobyte-admin-credentials.yaml
      ```

  3. Create `VolumeSnapshotContent` object for pre-provisioned volume with
   [required configuration](example/volume-snapshot-content-pre-provisioned.yaml)

        ```bash
        kubectl create -f example/volume-snapshot-content-pre-provisioned.yaml
        ```

  4. Create `VolumeSnapshot` object by adjusting the [example snapshot object](example/volume-snapshot-pre-provisioned.yaml)

     **name and namespace must match** `volumeSnapshotRef` **details** from the step 2

        ```bash
        kubectl create -f example/volume-snapshot-pre-provisioned.yaml
        ```

  5. (optional) verify created `volumesnapshot` and `volumesnapshotcontent` objects

        ```bash
        kubectl get volumesnapshot
        kubectl get volumesnapshotcontent
        ```

  6. [Restore snapshot](example/restore-snapshot-pvc-pre-provisioned.yaml)

        ```bash
        kubectl create -f example/restore-snapshot-pvc-pre-provisioned.yaml
        ```

  7. Create pod with [restored snapshot](example/nginx-demo-pod-with-pre-provisioned-snapshot-vol.yaml)

        ```bash
        kubectl create -f example/nginx-demo-pod-with-pre-provisioned-snapshot-vol.yaml
        ```

## Uninstall Quobyte CSI

1. Delete Quobyte CSI containers and corresponding RBAC

    List available helm charts

    ```bash
    helm list
    ```

    Delete intended chart

    ```bash
    helm delete <Quobyte-CSI-chart-name>
    ```

## Snapshotter Setup

### Install Snapshotter

The below setup is required once per k8s cluster

  ```bash
    kubectl create -f quobyte-csi-driver/k8s-snapshot-crd.yaml
    kubectl create -f quobyte-csi-driver/k8s-snapshot-controller.yaml

  ```

### Remove Snapshotter

  ```bash
    kubectl delete -f quobyte-csi-driver/k8s-snapshot-controller.yaml
    kubectl delete -f quobyte-csi-driver/k8s-snapshot-crd.yaml

  ```
