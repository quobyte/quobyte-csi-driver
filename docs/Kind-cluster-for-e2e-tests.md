# Quobyte CSI E2e tests

The aim of these set of scripts is to enable CSI e2e test runs against internal Quobyte cluster
(testing cluster)

## Requirements

1. Running docker service

2. Installed [kind tool](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).
   Make sure that installed location is part of your $PATH.

3. Quobyte testing cluster with user `csi-driver` and with password `quobyte` as admin of tenant `csi-test`

4. To run acess key tests, you must [import access kesys](./quobyte_access_keys.md)

    ```bash
      for file in example/access_keys/*.csv ; do qmgmt -u <QUOBYTE_API_URL> accesskey import "$file"; done
    ```

## Run tests

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
