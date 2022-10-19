# Shared Volume for Dynamic volumes

With this feature, you can provision PV as a sub-directory of the volume instead of an
exclusive Quobyte volume.

## Requirements

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
allowVolumeExpansion: true
parameters:
  quobyteTenant: "csi-test"
  # provisions PV as subdirectory of the "shared_volume"
  # Not having this parameter, triggers creation of a new Quobyte volume for the dynamic PV
  sharedVolumeName: "shared_volume"
  csi.storage.k8s.io/provisioner-secret-name: "quobyte-admin-credentials"
  csi.storage.k8s.io/provisioner-secret-namespace: "quobyte"
  csi.storage.k8s.io/controller-expand-secret-name: "quobyte-admin-credentials"
  csi.storage.k8s.io/controller-expand-secret-namespace: "quobyte"
  csi.storage.k8s.io/node-publish-secret-name: "quobyte-admin-credentials"
  csi.storage.k8s.io/node-publish-secret-namespace: "quobyte"
  quobyteConfig: "BASE"
  createQuota: "true"
  user: root
  group: root
  accessMode: "777"
reclaimPolicy: Delete
```
