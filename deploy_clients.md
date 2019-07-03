# Deploy Quobyte clients

Quobyte CSI driver requires Quobyte client to be installed with mount point `/mnt/quobyte/mounts`.

## Deploy quobyte-client package (systemd service) -- **Recommended**

1. Download install_quobyte script

```bash
wget https://support.quobyte.com/repo/3/<QUOBYTE_REPO_ID>/install_quobyte && chmod +x install_quobyte
```

2. Install client on (remote) node

`registry-endpoints` or `qns-id` can be found on registry nodes in `/etc/quobyte/registry.cfg` as `registry=<registry-endpoints>` and `qns.id=<qns-id>` respectively. 

```bash
sudo ./install_quobyte add-client --registry-endpoints <registry-endpoints> --mount-point /mnt/quobyte/mounts --repo-id <QUOBYTE_REPO_ID> [remote_user@remote_ip]
```

If your Quobyte deployment uses QNS, you should install client with `--qns-id`

```bash
sudo ./install_quobyte add-client --qns-id <qns-id> --mount-point /mnt/quobyte/mounts --repo-id <QUOBYTE_REPO_ID> [remote_user@remote_ip]
```

`Note:` 
1. `remote_user` must have sudo capabilities on `remote_ip` node to install Quobyte client.
2. install_quobyte uses `ssh` to install client on remote nodes. So, to install client on remote node,
 your base node must be able to connect remote node using `ssh`.

## Deploy Containerized Quobyte client

To use Quobyte volumes in Kubernetes, nodes must have a running Quobyte client
 with the mount point as `/mnt/quobyte/mounts`. Please see the
 [example client configuration](https://github.com/quobyte/quobyte-csi/blob/v1.0.0/example/client.yaml).

Label Kubernetes nodes

```bash
kubectl label nodes <node-1> <node-n> quobyte_client="true"
```

Edit `example/client.yaml` and configure `QUOBYTE_REGISTRY` environment variable, namespace.
 Deploy Quobyte clients.

```bash
kubectl create -f example/client.yaml
```

`Note:` Not deploying Quobyte clients with `/mnt/quobyte/mounts` results in pod start failures with mount fail errors.
