# Build from source code

To build Quobyte CSI, golang and docker (with containerd storage backend) must be installed
 on host machine.

1. Clone the Quobyte CSI codebase

    ```bash
    git clone git@github.com:quobyte/quobyte-csi-driver.git
    cd quobyte-csi-driver/src
    ```

5. Build release, publish the version and follow the instructions to make a release on github

    ```bash
    ./build release <RELEASE-VERSION>
    ```
