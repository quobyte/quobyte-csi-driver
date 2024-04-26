# Quobyte CSI logs collector

Log collector gathers logs from all the Quobyte CSI containers in single place for analysis.
 It also generates tar that can be uploaded with support ticket.  

1. Get the log_collector utility script on any node with working kubectl

    ```bash
    wget https://raw.githubusercontent.com/quobyte/quobyte-csi/master/log_collector.sh \
     && chmod +x log_collector.sh
     
    ```

2. Run the log_collector

    ```bash
    DRIVER_NAMESPACE="<quobyte-csi-driver-namespace>" ./log_collector.sh
    ```

    For example, if the Quobyte CSI driver is deployed in `quobyte-ns` then you need to run

    ```bash
    DRIVER_NAMESPACE="quobyte-ns" ./log_collector.sh
    ```

3. Logs can be found under the directory `./csi_logs` for analysis.

4. Script also generates the tar of `./csi_logs` as `quobyte_csi_logs.tar.gz`.
 Please upload the tar with Quobyte support ticket.
