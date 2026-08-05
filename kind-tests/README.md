# Quobyte CSI E2e tests

The aim of these set of scripts is to enable CSI e2e test runs against given k8s configuration
and Quobyte setup.

`run_test` provisions the kind cluster, then runs each self-contained Go test package under
[`e2e-tests/`](./e2e-tests). Every subdirectory of `e2e-tests/` (e.g.
[`e2e-tests/dynamic_provisioning/`](./e2e-tests/dynamic_provisioning)) that contains an `env` file
is treated as its own test: for each one found, `run_test`

1. **Deploys the environment** -- exports the variables in that directory's `env` file
   (`QUOBYTE_REGISTRY`, `ENABLE_ACCESS_KEY_MOUNTS`, `CSI_PROVISIONER_NAME`,
   `QUOBYTE_API_URL`/`QUOBYTE_API_USER`/`QUOBYTE_API_PASSWORD`/`QUOBYTE_TENANT`).
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

6. Quobyte API endpoint and registry endpoint

## Run tests

Run from the project root (`quobyte-csi-driver`):

```bash
kind-tests/cleanup
kind-tests/run_test
```

No Quobyte API credentials, registry endpoint, or driver `values.yaml` need to be passed on the
command line -- each test directory under `e2e-tests/` carries everything it needs:

- `env` -- `QUOBYTE_REGISTRY`, `ENABLE_ACCESS_KEY_MOUNTS`, `CSI_PROVISIONER_NAME`,
  `QUOBYTE_API_URL`, `QUOBYTE_API_USER`, `QUOBYTE_API_PASSWORD`, `QUOBYTE_TENANT`
- `values.yaml` -- Helm values for the `quobyte-csi` chart (`quobyte.dev.csiImage`/
  `quobyte.dev.podKillerImage`/`quobyte.dev.csiProvisionerVersion` are overridden by `run_test`
  with the locally built images)
- one or more `_test.go` files

Add a new test scenario by adding a new `e2e-tests/<name>/` directory with its own `env`,
`values.yaml`, and `_test.go` file.

To iterate on one test directly against an already-running cluster without rerunning all of
`run_test` (cluster creation, image build, etc.):

```bash
cd kind-tests/e2e-tests
set -a; source dynamic_provisioning/env; set +a
KUBECONFIG=/tmp/quobyte-k8s-config NAMESPACE=quobyte \
go test ./dynamic_provisioning/... -v -timeout 20m
```

## Cleanup

* To destroy `kind` cluster and other resources, run the following command
  (from project root: quobyte-csi-driver)

  ```bash
  kind-tests/cleanup
  ```
