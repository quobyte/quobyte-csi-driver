# Build from source code

To build Quobyte CSI, golang and docker must be installed on host machine.

1. Clone the Quobyte CSI codebase

    ```bash
    git clone git@github.com:quobyte/quobyte-csi.git
    cd quobyte-csi
    ```

2. Ensure required dependencies are downloaded

    ```bash
    dep ensure -v
    ```

3. Use `./build` utility to build the binary and push the container  
 
    To build binary

    ```bash
    ./build
    ```

    To build and push docker container

    ```bash
    ./build DOCKER_REPO_URL # Example, quay.io/quobyte/csi:v1.0.1
    ```
