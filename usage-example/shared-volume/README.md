# Shared Volume for Dynamic volumes

With this feature, you can provision PV as a sub-directory of the volume instead of an
exclusive Quobyte volume.

## Requirements

* Requires Quobyte client on Quobyte CSI controller pod (quobyte-csi-controller-....) running host
  with the [mount path](https://github.com/quobyte/quobyte-csi-driver/blob/v1.8.4/csi-driver-templates/values.yaml#L21)

* Shared volume(s) must be accessible on the Quobyte CSI controller node.

* For Quobyte 2.x cluster, your Quobyte CSI driver should be
  deployed with `quobyte.sharedVolumesList`. Quobyte CSI driver runs periodic cleanup of these
  volumes to release resources of deleted PVs. If you do not include/miss shared volumes, the
  Quobyte CSI driver does not run cleanup for the missing shared volume(s).

* For Quobyte 3.x cluster, the `quobyte.sharedVolumesList` can be ignored. As soon as the PV is
  deleted (depending on storage class retention policy), Quobyte CSI Driver triggers
  `DeleteFilesTask` for the directory of the shared volume.

## Storage Class configuration

Your storage class must provide `parameters.quobyteTenant` and `parameters.sharedVolumeName` along
with other parameters. Following is the sample storage class that uses `shared_volume` of the tenant
`csi-test`

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: quobyte-csi-shared-volume
# must match Quobyte CSI provisioner name
provisioner: csi.quobyte.com
# For shared volumes, volume expansion if requested from k8s always succeeds.
# admin need to either disable it via this flag or set Quota limits on shared volume.
#allowVolumeExpansion: true
parameters:
  quobyteTenant: "csi-test"
  # provisions PV as subdirectory of the "shared_volume"
  # Not having "sharedVolumeName" parameter, triggers creation of a new Quobyte volume
  # (with the name same as PV's name) for the dynamic provisioning
  sharedVolumeName: "shared_volume"
  # createQuota is not required with shared volumes and ignored if provided.
  # If Storage admin requires Quota for shared volumes, they must set Quota for volume via
  # Quobyte's management API at tenant level/volume level
  #createQuota: "true"
  csi.storage.k8s.io/provisioner-secret-name: "quobyte-admin-credentials"
  csi.storage.k8s.io/provisioner-secret-namespace: "quobyte"
  csi.storage.k8s.io/controller-expand-secret-name: "quobyte-admin-credentials"
  csi.storage.k8s.io/controller-expand-secret-namespace: "quobyte"
  csi.storage.k8s.io/node-publish-secret-name: "quobyte-admin-credentials"
  csi.storage.k8s.io/node-publish-secret-namespace: "quobyte"
  user: root
  group: root
  accessMode: "750"
reclaimPolicy: Delete
```
