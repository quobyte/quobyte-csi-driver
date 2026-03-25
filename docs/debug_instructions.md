# Debugging Driver issues

## Attach Debug container to Quobyte CSI Driver Pod

Debug container can be added to the Quobyte CSI driver as following
```bash
kubectl -n <csi-driver-namespace> debug -it <quobyte-csi-pod> --image=ubuntu \
  --target=quobyte-csi-driver --share-processes=true --profile sysadmin
```

Find CSI driver process using `ps -aux`

Navigate to root file system of the `quobyte-csi-driver` container using `cd /proc/<pid>/root/`.
For example, you can list Quobyte volumes available via client mount points using 
`ls /proc/<pid>/root/<client-mount-point>/mount`