# Sanity tests

Quobyte's own checks of the CSI driver. Each subdirectory is one self-contained Go test
package laid out like [`dynamic_provisioning/`](./dynamic_provisioning):

```
<scenario>/
  <scenario>_test.go   one or more Go tests
  env/
    <name>             one driver setup; the package is run once per file
```

`run_test` discovers them by that `env/` directory and names each combination
`<scenario>/<env file>` — see [../README.md](../README.md) for the env file contract and
how to run a single combination.

## Scenarios

| Directory | Covers | State |
| --- | --- | --- |
| [`dynamic_provisioning/`](./dynamic_provisioning) | A PVC gets its own Quobyte volume, mounted read/write, and the volume really exists; also the StorageClass parameters `user`, `group`, `labels`, `accessMode` and `createQuota`, including the rollback when the quota is refused | implemented |
| [`pre_provisioned_volumes/`](./pre_provisioned_volumes) | A PV pointed at a volume the driver did not create; nothing calls CreateVolume and Retain leaves the volume alone | implemented |
| [`shared_volume/`](./shared_volume) | Two claims become subdirectories of one volume, isolated from each other; deleting one leaves the other and the volume, and `accessMode` reaches the subdirectory | implemented |
| [`shared_volume_cleanup/`](./shared_volume_cleanup) | The other branch of shared volume cleanup: with the delete files task off, a deleted claim's subdirectory is renamed to the driver's delete marker rather than removed | implemented, see the note below |
| [`expansion/`](./expansion) | Growing a claim moves the Quobyte quota, and does nothing at all for a claim inside a shared volume | implemented |
| [`immediate_erase/`](./immediate_erase) | With `immediateErase`, the volume is gone from its tenant shortly after the PV is, instead of lingering | implemented |
| [`multi_node_access/`](./multi_node_access) | One ReadWriteMany volume mounted from two workers at once, each pod reading what the other wrote | implemented |
| [`volume_metrics/`](./volume_metrics) | `kubelet_volume_stats_*` appear for a claim with metrics on and not with them off, and the volume mounts either way | implemented |
| [`subdirectory/`](./subdirectory) | A three-part volume handle mounts a subdirectory and nothing above it | implemented |
| [`namespace_tenant_mapping/`](./namespace_tenant_mapping) | Which tenant a volume lands in: the PVC's namespace, the Secret's access key, or the StorageClass — which always overrides the other two | implemented, see the note below |
| [`access_key_negative/`](./access_key_negative) | Wrong and missing credentials fail visibly — at mount time and at provisioning time respectively — and a valid key cannot reach another tenant | implemented |
| [`pod_killer/`](./pod_killer) | A pod whose mount went stale is deleted and comes back on a working mount | implemented |
| [`snapshots/`](./snapshots) | Snapshot and restore | TODO |

Three things are deliberately not covered yet:

- **a tenant nothing names** is left out of `namespace_tenant_mapping/`. Its four env files
  cross access key mounts with the namespace mapping, and the case without a `quobyteTenant`
  in the StorageClass runs in three of them; in the fourth (user/password, no mapping)
  nothing names a tenant at all, the driver sends none, and whether the API still finds one
  is undefined — so only the StorageClass override is asserted there. The full matrix is in
  [`namespace_tenant_mapping/README.md`](./namespace_tenant_mapping/README.md).

- **snapshots** is structure only — the directory, its `env/` file and a test that says what
  it has to cover, then calls `t.Skip`. It needs the VolumeSnapshot CRD types, which means
  adding `github.com/kubernetes-csi/external-snapshotter/client/v8` to
  [`../go.mod`](../go.mod). Note that **a skipped test passes**: its row in the summary table
  reads `PASS` while proving nothing, and `run_test` still deploys a driver for it.
- **the sweep that removes a shared volume's delete markers** is uncovered.
  `shared_volume_cleanup/` asserts the rename half of that branch — with the delete-files
  task off, a deleted claim's subdirectory becomes `<driverName>_delete_<subdir>` and still
  holds its data — but not that the marker is ever collected. The collector only walks
  volumes named in `--shared_volumes_list`, which takes volume *UUIDs*, and the UUID of a
  per-run volume cannot be known when the helm values are set; it also first runs five
  minutes after the controller starts. Covering it needs a volume that outlives the run,
  with its UUID pinned in the env file.

Use `TESTS` to work on one scenario at a time, e.g.
`TESTS='shared_volume' kind-tests/run_test --sanity http://host:port host:port` — naming the
package runs every environment of it, and stops at the directory boundary, so that one does not
also run `shared_volume_cleanup/`.
