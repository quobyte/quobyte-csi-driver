# Quobyte CSI Helm Chart

## Requirements

* Kubernetes >= v1.22.x
* helm

## Install Quobyte CSI Driver

1. Add Quobyte CSI Driver to your helm repositories

    ```bash
    helm repo add quobyte-csi-driver https://quobyte.github.io/quobyte-csi-driver/helm
    ```

2. List all available versions

    ```bash
    helm search repo quobyte-csi-driver/quobyte-csi-driver -l
    ```

3. Explore version with helm

    ```bash
    helm show <helm> quobyte-csi-driver/quobyte-csi-driver --version <csi-driver-version>
    ```

4. List available configuration options of the chosen Quobyte CSI driver version

    ```bash
       helm show values quobyte-csi-driver/quobyte-csi-driver --version <csi-driver-version>
    ```

5. Install Quobyte CSI driver by overriding values with `-f <your-configured-values.yaml>`

    ```bash
      helm install ... -f <your-configured-values.yaml> --version <csi-driver-version>
    ```
