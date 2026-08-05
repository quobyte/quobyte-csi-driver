# Quobyte CSI E2e tests

The aim of these set of scripts is to enable CSI e2e test runs against given k8s configuration
and Quobyte setup.

`run_test` provisions the kind cluster, then runs each self-contained Go test package under
[`e2e-tests/`](./e2e-tests). Every subdirectory of `e2e-tests/` (e.g.
[`e2e-tests/dynamic_provisioning/`](./e2e-tests/dynamic_provisioning)) that contains an `env` file
is treated as its own test: for each one found, `run_test`

1. **Deploys the environment** -- exports the variables in that directory's `env` file
   (`ENABLE_ACCESS_KEY_MOUNTS` and any test-specific overrides like `QUOBYTE_TENANT`). The
   Quobyte API endpoint/client registry (`QUOBYTE_API_URL`/`QUOBYTE_API_USER`/
   `QUOBYTE_API_PASSWORD`/`QUOBYTE_REGISTRY`) and `CSI_PROVISIONER_NAME` are not part of
   the per-test `env` file -- they're script-level config set by `run_test` itself (see
   Requirements below), since they're the same physical endpoints/driver for every test
   directory in a run.
2. Deploys the Quobyte client via the [`quobyte-client`](./quobyte-k8s-resources/helm/quobyte-client)
   helm chart using `QUOBYTE_REGISTRY`/`ENABLE_ACCESS_KEY_MOUNTS`, and the CSI driver (built from
   source) via the [`quobyte-csi`](./quobyte-k8s-resources/helm/quobyte-csi) helm chart using that
   same directory's `values.yaml` and `CSI_PROVISIONER_NAME`.
3. **Runs the test** -- `go test ./<name>/...` for just that package.
4. Tears down both helm releases.
5. **Removes the environment** -- unsets the `env` file's variables before moving to the next test
   directory.

Tests build their own Secret/StorageClass/PVC/Pod in Go (uniquely named per run), write/read a file
through the pod's Quobyte mount, and talk directly to the Quobyte API to confirm the backing volume
was actually created -- so results are asserted by `go test`, not eyeballed.

There is no other supported flow: the old YAML-driven `test-configs/` setup and the upstream
sig-storage ginkgo suite it drove are no longer used.

## Requirements

1. Running docker service

2. Installed [kind tool](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).
   Make sure that installed location is part of your $PATH.

3. Installed `helm` tool

4. Installed `kubectl`

5. Installed `go`

6. Quobyte API endpoint and registry endpoint, passed to `run_test` as positional
   arguments: `<QUOBYTE_API_URL> <QUOBYTE_REGISTRY> [QUOBYTE_API_USER] [QUOBYTE_API_PASSWORD]`.
   The URL and registry are required; user/password are optional and default to
   `admin`/`quobyte` if omitted. `CSI_PROVISIONER_NAME` also defaults to `csi.quobyte.com`
   inside `run_test`; override by pre-setting the env var if a different provisioner name
   is needed.

## Run tests

Run from the project root (`quobyte-csi-driver`):

```bash
kind-tests/cleanup
kind-tests/run_test http://host:port host:port
# or, overriding the admin/quobyte user/password defaults:
kind-tests/run_test http://host:port host:port myuser mypassword
```

`run_test` requires the Quobyte API endpoint and client registry as its first two
arguments (see Requirements above) -- it `die`s immediately with a usage message if
either is missing. The API user/password are optional, defaulting to `admin`/`quobyte`.
No driver `values.yaml` needs to be passed on the command line -- each test directory
under `e2e-tests/` carries everything else it needs:

- `env` -- `ENABLE_ACCESS_KEY_MOUNTS` and any test-specific overrides (e.g.
  `QUOBYTE_TENANT` to pin a pre-existing tenant instead of `run_test`'s generated per-run
  name)
- `values.yaml` -- Helm values for the `quobyte-csi` chart (`quobyte.dev.csiImage`/
  `quobyte.dev.podKillerImage`/`quobyte.dev.csiProvisionerVersion` are overridden by `run_test`
  with the locally built images)
- one or more `_test.go` files

Add a new test scenario by adding a new `e2e-tests/<name>/` directory with its own `env`,
`values.yaml`, and `_test.go` file.

Before deleting the Secret/StorageClass it creates, each test writes a standalone,
`kubectl apply`-able copy of them to `$ARTIFACTS_DIR` (if set), named
`<TestName>-<suffix>-secret.yaml` / `-storageclass.yaml`. `run_test` points this at
`${debug_dir}/artifacts` for each test run and uses the dumped StorageClass/Secret to run
the upstream Kubernetes e2e (external-storage) suite against the exact same setup, after
the Go test's own cleanup has already deleted the live objects.

To iterate on one test directly against an already-running cluster without rerunning all of
`run_test` (cluster creation, image build, etc.):

```bash
cd kind-tests/e2e-tests
set -a; source dynamic_provisioning/env; set +a
KUBECONFIG=/tmp/quobyte-k8s-config NAMESPACE=quobyte \
QUOBYTE_API_URL=http://host:port QUOBYTE_API_USER=admin QUOBYTE_API_PASSWORD=secret \
CSI_PROVISIONER_NAME=csi.quobyte.com \
go test ./dynamic_provisioning/... -v -timeout 20m
```

## Cleanup

* To destroy `kind` cluster and other resources, run the following command
  (from project root: quobyte-csi-driver)

  ```bash
  kind-tests/cleanup
  ```
