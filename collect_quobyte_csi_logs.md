# Quobyte CSI logs collector

Log collector gathers logs from all the Quobyte CSI containers in single place for analysis.
 It also generates tar that can be uploaded with support ticket.  

1. Get the log_collector utlity script on any K8S node with working kubectl

```bash
wget https://raw.githubusercontent.com/quobyte/quobyte-csi/v1.0.0/log_collector && chmod +x log_collector
```

2. Run the log_collector

```bash
./log_collector
```

3. Logs can be found under directory `./csi_logs` for analysis.

4. Script also generates tar of `./csi_logs` as `quobyte_csi_logs.tar.gz`.
 Please upload tar with the support ticket.
