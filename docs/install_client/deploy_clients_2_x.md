# Deploy Quobyte clients

Quobyte CSI driver requires a running Quobyte client with the mount point <[values.clientMountPoint](https://github.com/quobyte/quobyte-csi/blob/4671450b0dec5fe162f78f9e35c6c6fe90e3f86b/quobyte-csi-driver/values.yaml#L18)>`/mounts` on every host node. This example deployment of clients assumes
`values.clientMountPoint: /mnt/quobyte`

## Deploy quobyte-client package (systemd service) -- **Recommended**

1. Download the install_quobyte script

    ```bash
    wget https://support.quobyte.com/repo/3/<QUOBYTE_REPO_ID>/install_quobyte \
     && sudo chmod +x install_quobyte
    ```

2. Install client on (remote) node

    `registry-endpoints` or `qns-id` can be found on registry nodes in `/etc/quobyte/registry.cfg` as `registry=<registry-endpoints>` and `qns.id=<qns-id>` respectively.

    ```bash
    sudo ./install_quobyte add-client --registry-endpoints <registry-endpoints> \
     --mount-point /mnt/quobyte/mounts --repo-id <QUOBYTE_REPO_ID> \
     [remote_user@remote_ip]
    ```

    If your Quobyte deployment uses QNS, you should install client with `--qns-id`

    ```bash
    sudo ./install_quobyte add-client --qns-id <qns-id> \
     --mount-point /mnt/quobyte/mounts --repo-id <QUOBYTE_REPO_ID> \
      [remote_user@remote_ip]
    ```

    To use access keys, `quobyte-client.service` must be started with `--enable-access-contexts`.

`Note:`  

1. `remote_user` must have sudo capabilities on the `remote_ip` node to install Quobyte client.

2. install_quobyte uses `ssh` to install the Quobyte client on the remote node. So, to install client on a remote node,
 your base node must be able to connect the remote node using `ssh`.

## Deploy Containerized Quobyte client

To use Quobyte volumes in Kubernetes, nodes must have a running Quobyte client
 with the mount point as `/mnt/quobyte/mounts`. Please see the
 [example client configuration](https://github.com/quobyte/quobyte-csi/blob/v1.0.1/example/client.yaml).

1. Label Kubernetes nodes

    ```bash
    kubectl label nodes <node-1> <node-n> quobyte_client="true"
    ```

2. Edit `example/client.yaml` and configure

    * `namespace` of your choice
    * `QUOBYTE_REGISTRY` environment variable set with Quobyte registry
    * `QUOBYTE_MOUNT_POINT` environment variable set to `/mnt/quobyte/mounts`
    * host path volume `/mnt/quobyte`  

3. Deploy Quobyte clients.

    ```bash
    kubectl create -f example/client.yaml
    ```

  `Note:` Not deploying Quobyte clients with `/mnt/quobyte/mounts` results in pod start failures with mount fail errors.
