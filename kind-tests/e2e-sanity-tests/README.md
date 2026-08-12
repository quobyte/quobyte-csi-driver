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
| [`dynamic_provisioning/`](./dynamic_provisioning) | A PVC gets its own Quobyte volume, mounted read/write, and the volume really exists | implemented |
| [`pre_provisioned_volumes/`](./pre_provisioned_volumes) | A PV pointed at a volume the driver did not create; nothing calls CreateVolume and Retain leaves the volume alone | implemented |
| [`shared_volume/`](./shared_volume) | Two claims become subdirectories of one volume, isolated from each other; deleting one leaves the other and the volume | implemented, see the note below |
| [`subdirectory/`](./subdirectory) | A three-part volume handle mounts a subdirectory and nothing above it | implemented |
| [`namespace_tenant_mapping/`](./namespace_tenant_mapping) | Which tenant a volume lands in: the PVC's namespace, the Secret's access key, or the StorageClass — which always overrides the other two | implemented, see the note below |
| [`access_key_negative/`](./access_key_negative) | Wrong and missing credentials fail visibly — at mount time and at provisioning time respectively | implemented |
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
- **shared volume cleanup** (the delete-files task vs the client rm walk) is not covered by
  `shared_volume/`. The driver only cleans up volumes named in `--shared_volumes_list`,
  which takes volume *UUIDs*, and the UUID of a per-run volume cannot be known when the helm
  values are set. Covering it needs a volume that outlives the run, with its UUID pinned in
  the env file.

Use `TESTS` to work on one scenario at a time, e.g.
`TESTS='shared_volume/*' kind-tests/run_test --sanity http://host:port host:port`.
