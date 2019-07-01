# Common mistakes/errors

1. **Issue:** Unsupported protocol ""  
   **Cause:** `quobyte.apiURL` not configured correctly in `deploy/config.yaml`  
   **Solution:** Redeploy `deploy/config.yaml` with valid `quobyte.apiURL`. Follwed by delete the
   Quobyte CSI driver and redeploy again.

2. **Issue:** Pod with Quobyte volume fails to start  
   **Cause:** Quobyte client is not running on node or
          Quobyte client does not use `/mnt/quobyte/mounts` as the mountpoint.  
   **Solution:** Redeploy Quobyte clients with `/mnt/quobyte/mounts` mountpoint.

