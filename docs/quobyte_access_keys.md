# Secure Storage Access with Quobyte Access Keys

Quobyte CSI requires Quobyte Management API access. The API access can be granted with user
credentials (username/password) or `API and Webconsole` access key. Further, (optionally) you can
protect volume mount from unexpected/malicious access with `File System` access key.

## Requirements

Requires Quobyte version 3.1 or later

To enable volume mount protection:

1. Quobyte client(s) must be deployed with `--enable-access-contexts` and
  `--no-default-permissions` options (see [example client](../example/client.yaml))
2. Quobyte CSI driver must be deployed with `enableAccessKeyMounts: true`
3. Requires `csi-test` tenant and user `csi-driver` as member of tenant `csi-test`. Additionally,
   `csi-driver` user must have a primary group.

## Storage Access with Access Keys

The following examples use imported Quobyte access keys and should only be used for testing.
For production usage, you should create relevant access keys through
 Quobyte web console -> My Quobyte -> My Access Keys or other means such as qmgmt, management API
 and then update your secrets with the access key information.

To import access keys, you need `qmgmt` available on the node. Additionally, you need to set
`API_URL` environment variable with Quobyte API Url.

```bash
API_URL="<your-quobyte-cluster-api-url>"
```

### Separate Management and File System Access Keys

* Import [Quobyte API access key](../example/access_keys/api_access_keys.csv) into your Quobyte Cluster

    ```bash
    qmgmt -u $API_URL accesskey import example/access_keys/api_access_keys.csv
    ```

* Create [API secret](../example/access_keys/quobyte-api-secret.yaml) with the imported
   API access key information

    ```bash
    kubectl apply -f example/access_keys/quobyte-api-secret.yaml
    ```

* Import [Quobyte mount/file system access key](../example/access_keys/mount_access_keys.csv) into
   your Quobyte Cluster

    ```bash
    qmgmt -u $API_URL accesskey import example/access_keys/mount_access_keys.csv
    ```

* Create [mount secret](../example/access_keys/quobyte-mount-secret.yaml) with the imported
   mount access key information

    ```bash
    kubectl apply -f example/access_keys/quobyte-mount-secret.yaml
    ```

* Create a [storage class](../example/access_keys/storage-class-api-and-mount-secret.yaml) with the `quobyte-api-secret` and `quobyte-mount-secret` secrets

    ```bash
    kubectl apply -f example/access_keys/storage-class-api-and-mount-secret.yaml
    ```

* Create [PVC](../example/access_keys/pvc-api-and-mount-secret.yaml) with the storage class `api-and-mount-secret-storage-class`
 access keys

    ```bash
    kubectl apply -f example/access_keys/pvc-api-and-mount-secret.yaml
    ```

* Create [Nginx pod](../example/access_keys/nginx-api-and-mount-secret.yaml) using the above PVC

    ```bash
    kubectl apply -f example/access_keys/nginx-api-and-mount-secret.yaml
    ```

* Once the pod is running, copy the [index file](../example/index.html) to the deployed nginx pod

    ```bash
    kubectl cp example/index.html nginx-api-and-mount-secret:/usr/share/nginx/html/
    ```

* Access the home page served by nginx pod from the command line

    ```bash
    curl http://$(kubectl get pods nginx-api-and-mount-secret -o yaml | grep ' podIP:' | awk '{print $2}'):80
    ```

### Single Access Key for both API and File System Access

* Import [Quobyte All uses access key](../example/access_keys/all_uses_access_key.csv) into your Quobyte
 Cluster

    ```bash
    qmgmt -u $API_URL accesskey import example/access_keys/all_uses_access_keys.csv
    ```

* Create a [secret](../example/access_keys/quobyte-generic-secret.yaml) with the imported
  API access key information

    ```bash
    kubectl create -f example/access_keys/quobyte-generic-secret.yaml
    ```

* Create the [storage class](example/access_keys/storage-class-generic-secret.yaml) with the `quobyte-generic-secret` secret

    ```bash
    kubectl apply -f example/access_keys/storage-class-generic-secret.yaml
    ```

  * Create [PVC](../example/access_keys/pvc-generic-secret.yaml) with the storage class `api-and-mount-secret-storage-class`
 access keys

    ```bash
    kubectl apply -f example/access_keys/pvc-generic-secret.yaml
    ```

* Create [Nginx pod](../example/access_keys/nginx-generic-secret.yaml) using the above PVC

    ```bash
    kubectl apply -f example/access_keys/nginx-generic-secret.yaml
    ```

* Once the pod is running, copy the [index file](example/index.html) to the deployed nginx pod

    ```bash
    kubectl cp example/index.html nginx-generic-secret:/usr/share/nginx/html/
    ```

* Access the home page served by nginx pod from the command line

    ```bash
    curl http://$(kubectl get pods nginx-generic-secret -o yaml | grep ' podIP:' | awk '{print $2}'):80
    ```

**NOTE**:

* If your k8s secret contains `user:` and `password:`, Quobyte CSI driver uses this information
 to access Quobyte management API.

* If tenant-name/volume-name is provided for pre-provisioned volume PV, you must provide "all uses"
  access key as mount secret. Alternatively, you could use volume-uuid and more restrictive
  "file system/mount" access key in the secret.
