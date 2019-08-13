# Common mistakes/errors

1. **Issue:** Unsupported protocol ""  
   **Cause:** `quobyte.apiURL` not configured correctly in `deploy/config.yaml`  
   **Solution:** Redeploy `deploy/config.yaml` with valid `quobyte.apiURL`. Follwed by delete the
   Quobyte CSI driver and redeploy again.

2. **Issue:** Pod with Quobyte volume fails to start  
   **Cause:** Quobyte client is not running on node or
          Quobyte client does not use `/mnt/quobyte/mounts` as the mountpoint.  
   **Solution:** Redeploy Quobyte clients with `/mnt/quobyte/mounts` mountpoint by following
    [client deployment instructions](deploy_clients.md).  

3. **Issue:** Quobyte CSI Pods are not starting afer enabling `PodSecurityPolicy`  
   **Cause:** `ServiceAccounts` used by Quobyte CSI driver does not have required PSPs created.  
   **Solution:** Redeploy Quobyte CSI driver using `deploy/deploy-csi-driver-1.0.1-k8sv1.14-PSP.yaml`

