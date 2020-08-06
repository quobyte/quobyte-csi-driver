# Multi-cluster setup

**Quobyte access keys are highly recommended for multi-cluster deployments**

Quobyte CSI Driver supports access to mutliple Quobyte storage clusters
 (for example, production and testing) from single k8s cluster.

To use one Quobyte cluster, you always need one Quobyte CSI driver
 i.e. one-to-one mapping between Quobyte CSI driver and Quobyte cluster.
  To use `n` Quobyte clusters, you need `n` Quobyte CSI drivers.

For each driver the following must be met

1. Unique `quobyte.apiURL` in [driver configuration](../quobyte-csi-driver/values.yaml)

2. Unique `quobyte.csiProvisionerName` in [driver configuration](../quobyte-csi-driver/values.yaml)

3. Unique `quobyte.clientMountPoint` in [driver configuration](../quobyte-csi-driver/values.yaml)

4. k8s nodes with multiple Quobyte clients, each with their own mountPoint and registry configuration

Adjust the driver configuration and deploy driver with `helm install <SOME_UNIQUE_NAME> ./quobyte-csi-driver`

The value configured for `quobyte.csiProvisionerName` must be used as `StorageClass.provisioner` to refer
 this Quobyte Cluster/CSI driver combination

## Installation of mulitple native clients

### Limitations

  1. Two native clients with different version cannot be installed on same machine
  2. `install_quobyte add-client` can only install single client on a machine, additional
    clients must be installed/cloned manually
  
### Installation of clients (for systemd services)

1. Install Quobyte client following [client installation instructions](deploy_clients.md)

2. Clone copies of service and configuration

    ```bash
    clusters="test1 test2" # cluster names separated by single space
    for cluster in $clusters;
    do
      sudo cp /usr/lib/systemd/system/quobyte-client.service /usr/lib/systemd/system/quobyte-client-$cluster.service
      sudo sed -i -e "s/EnvironmentFile=\/etc\/quobyte\/client-service.env/EnvironmentFile=\/etc\/quobyte\/client-service-${cluster}.env/g" /usr/lib/systemd/system/quobyte-client-$cluster.service
      sudo systemctl enable /usr/lib/systemd/system/quobyte-client-$cluster.service
      sudo cp /etc/quobyte/client-service.env /etc/quobyte/client-service-$cluster.env
      sudo sed -i -e "s/config_file=\/etc\/quobyte\/client-service.cfg/config_file=\/etc\/quobyte\/client-service-$cluster.cfg/g" /etc/quobyte/client-service-$cluster.env
      sudo cp /etc/quobyte/client-service.cfg /etc/quobyte/client-service-$cluster.cfg
    done
    ```

3. Edit `/etc/quobyte/client-service<-YOUR_CLUSTER_NAME>.cfg` and configure registry, client mount points (as needed by CSI driver).

4. Start clients

    ```bash
    clusters="test1 test2" # cluster names separated by single space
    for cluster in $clusters;
    do
      sudo systemctl start quobyte-client-$cluster.service
      sudo systemctl status quobyte-client-$cluster.service
    done
    ```

5. Remove clients

    ```bash
    remove_cluster_client="test1 test2"  # cluster names separated by single space
    for cluster in $remove_cluster_client;
    do
      sudo systemctl stop quobyte-client-$cluster.service
      sudo rm -f /usr/lib/systemd/system/quobyte-client-$cluster.service
      sudo rm -f /etc/systemd/system/multi-user.target.wants/quobyte-client-$cluster.service
      sudo rm -f /etc/systemd/system/remote-fs.target.wants/quobyte-client-$cluster.service
      sudo rm -f /etc/quobyte/client-service-$cluster.env
      sudo rm -f /etc/quobyte/client-service-$cluster.cfg
    done
    ```
