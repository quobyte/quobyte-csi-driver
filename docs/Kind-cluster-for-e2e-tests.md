# Quobyte CSI Deploy Scripts

Aim of this set of scripts is setting up [Requirements](https://github.com/quobyte/quobyte-csi#requirements), [Deploy Quobyte clients 3.x](https://github.com/quobyte/quobyte-csi/blob/master/docs/install_client/deploy_clients_3_x.md), [Deploy Quobyte CSI](https://github.com/quobyte/quobyte-csi#deploy-quobyte-csi-driver) and [Use Quobyte volumes in Kubernetes](https://github.com/quobyte/quobyte-csi#use-quobyte-volumes-in-kubernetes)

Scripts are developed to be executed against `testing` environment and as far as it requires encription-in-transit, you will need to add valid `ca.pem`, `client-cert.pem` and `client-key.pem` files to the working directory, so they will be used on Quobyte Clients mount stage.

## Requirements

1. Running docker service

2. Installed [kind tool](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).
   Make sure that installed location is part of your $PATH.

3. Quobyte testing cluster with user `csi-driver` and with password `quobyte` as admin of tenant `csi-test`

4. To run acess key tests, you must [import access kesys](./quobyte_access_keys.md)

    ```bash
      for file in example/access_keys/*.csv ; do qmgmt -u <QUOBYTE_API_URL> accesskey import "$file"; done
    ```

## Run scripts in the following order

1. Clone `quobyte-csi` repo & checkout a feature branch

    ```bash
    git clone <https://github.com/quobyte/quobyte-csi.git> && cd quobyte-csi && git checkout <branch/commit>
    ```

2. Copy your `ca.pem`, `client-cert.pem`, and `client-key.pem` into the `quobyte-csi/kind-cluster`

3. Run k8s e2e tests

    **NOTE**: Test execution updates some CSI file definitions, make sure your local changes are committed
             or staged before triggering of the tests

    * Using management API username and password

        ```bash
            (git checkout -- . && git clean -f; kind-cluster/delete_cluster && sleep 3m; kind-cluster/setup_k8s_cluster && docker exec -it $(docker ps -aqf "name=control-plane") bash -x /quobyte-csi/kind-cluster/install_csi_3x install && docker exec -it $(docker ps -aqf "name=control-plane") bash -x /quobyte-csi/kind-cluster/pre-flight_checks &&  docker exec -it $(docker ps -aqf "name=control-plane") bash -x /quobyte-csi/kind-cluster/install_csi_3x e2e) | tee $(mktemp tests-XXXXXX)
        ```

    * Using [access keys](../example/access_keys)

        ```bash
            (git checkout -- . && git clean -f; kind-cluster/delete_cluster && sleep 3m; MOUNT_WITH_ACCESS_KEYS='y' kind-cluster/setup_k8s_cluster && docker exec -it $(docker ps -aqf "name=control-plane") bash -x /quobyte-csi/kind-cluster/install_csi_3x install && docker exec -it $(docker ps -aqf "name=control-plane") bash -x /quobyte-csi/kind-cluster/install_csi_3x e2e) | tee $(mktemp tests-XXXXXX)
        ```

## Script's revert options

* To revert changes introduced by running tests

    **NOTE**: make sure the reverting changes only includes changes made by script

    ```bash
    git checkout -- . && git clean -f
    ```

* To revert pre-flight_checks

    ```bash
    docker exec -it $(docker ps -aqf "name=control-plane") bash /quobyte-csi/kind-cluster/pre-flight_checks
    ```

* To uninstall Quobyte CSI driver - use `./install_csi_3x uninstall`

    ```bash
    docker exec -it $(docker ps -aqf "name=control-plane") bash /quobyte-csi/kind-cluster/install_csi_3x uninstall
    ```

* To delete kind cluster and all related resources - execute `kind-cluster/delete_cluster` on your host machine (the script is not deleting created docker image)
