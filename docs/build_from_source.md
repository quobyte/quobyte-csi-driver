# Build from source code

To build Quobyte CSI, golang and docker must be installed on host machine.

1. Clone the Quobyte CSI codebase

    ```bash
    git clone git@github.com:quobyte/quobyte-csi.git
    cd quobyte-csi
    ```

2. Use `./build` utility to build the binary and push the container (during development) 

    To build binary

    ```bash
    ./build
    ```

    To build and push docker container

    ```bash
    # Pushes container to quay.io/quobyte/csi with given release version
    # Use -pre
    ./build container RELEASE_VERSION # example, ./build container v1.0.1-pre
    ```

    or use alternate repository

    ```bash
    # Pushes container to custom docker registry my-registry.io/quobyte/csi with given release version
    # example, CONTAINER_URL_BASE="my-registry.io/quobyte/csi:"./build container v1.0.1-pre
    CONTAINER_URL_BASE="URL_BASE:" ./build container RELEASE_VERSION
    ```

3. (if not installed) Get helm

    ```bash
    (cd /tmp && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 \
        && chmod 700 get_helm.sh && ./get_helm.sh)
    ```

4. Update [values.yaml](../quobyte-csi-driver/values.yaml) appropriate values, with built image in step 2 and deploy quobyte-csi driver

    ```bash
    helm install quobyte-csi ./quobyte-csi-driver
    ```

5. Deploy Quobyte clients and test with [e2e tests](e2e) **not completely automated, look inside the file for instructions**
  
    Running e2e tests require k8s cluster, please set it up with [kubespray](https://github.com/kubernetes-sigs/kubespray). Edit Vagrantfile pf the cloned repo and increase resources (cpus, memory). The default 3 node setup is sufficient to run e2e tests.

6. Build release and publish the version (merge change onto master and make release on merged master) on
 github by following post build instructions

    ```bash
    ./build release RELEASE-VERSION
    ```

## Update dependency

1. Get the dependency (example) and tidy old dependencies

    ```bash
    go get github.com/quobyte/api@[version/commit] && go mod tidy
    ```

2. Build with build script
