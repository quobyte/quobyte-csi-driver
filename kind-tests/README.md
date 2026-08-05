# Quobyte CSI E2e tests

The aim of these set of scripts is to enable CSI e2e test runs against given k8s configuration
and Quobyte setup.

`run_test` provisions the kind cluster, deploys the CSI driver (built from source) via the
[`quobyte-csi`](./quobyte-k8s-resources/helm/quobyte-csi) helm chart, and deploys the Quobyte
client via the [`quobyte-client`](./quobyte-k8s-resources/helm/quobyte-client) helm chart --
`quobyte.registry` and `quobyte.enableAccessKeys` come from the `QUOBYTE_REGISTRY` (required) and
`ENABLE_ACCESS_KEY_MOUNTS` (optional, defaults to `false`) environment variables, passed to `helm
install` as `--set` overrides. It then runs tests in one of two modes:

- **Legacy (default)**: applies the `k8s_*.yaml` manifests in `TEST_CASE_DIR`
  (StorageClass, Secret) and, if a `k8s_storage_class.yaml` is present, runs the upstream
  sig-storage "external storage" ginkgo suite via [`kind-tests/e2e`](./e2e). Results need manual
  verification.
- **Go e2e suite** (`RUN_GO_E2E_TESTS=true`): each subdirectory of [`e2e-tests/`](./e2e-tests)
  (e.g. `e2e-tests/dynamic_provisioning/`) is a self-contained Go test package with its own `env`
  file. For each one found, `run_test` deploys its environment (exports the file's variables),
  runs just that package's tests, then removes the environment (unsets those variables) before
  moving to the next. Tests build their own Secret/StorageClass/PVC/Pod in Go (uniquely named per
  run), write/read a file through the pod's Quobyte mount, and talk directly to the Quobyte API to
  confirm the backing volume was actually created -- so results are asserted by `go test`, not
  eyeballed.

## Requirements

1. Running docker service

2. Installed [kind tool](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).
   Make sure that installed location is part of your $PATH.

3. Installed `helm` tool

4. Installed `kubectl`

5. Quobyte API endpoint and registry endpoint

## Run tests

1. Setup your test following [test example](./test-configs/)

2. Run your test with command (from project root - quobyte-csi-driver)

    ```bash
    kind-tests/cleanup; \
    QUOBYTE_REGISTRY=<host>:<port> ENABLE_ACCESS_KEY_MOUNTS=false \
    TEST_CASE_DIR="<absolute-path-to-your-test-case-dir>" kind-tests/run_test
    ```
  
    or

    You can also run `kind-tests/run_test` without `TEST_CASE_DIR` to provision a kubernetes cluster
    . Thereafter, you could `export KUBECONFIG=...` as instructed by script output and install
    csi driver, execute tests manually.

    or

    You can run with `TEST_CASE_DIR` that contains only CSI driver values.yaml to deploy the driver
    (note that some defined values such as CSI image/pod killer images are overridden)

3. To run the Go e2e suite instead of the legacy flow, set `RUN_GO_E2E_TESTS=true`:

    ```bash
    kind-tests/cleanup
    QUOBYTE_REGISTRY=<host>:<port> RUN_GO_E2E_TESTS=true \
    TEST_CASE_DIR="<absolute-path-to-your-test-case-dir>" kind-tests/run_test
    ```

   No Quobyte API credentials need to be passed on the command line -- each test directory under
   `e2e-tests/` (e.g. [`e2e-tests/dynamic_provisioning/`](./e2e-tests/dynamic_provisioning)) carries
   its own `env` file with the `QUOBYTE_API_URL`/`QUOBYTE_API_USER`/`QUOBYTE_API_PASSWORD`/
   `QUOBYTE_TENANT`/`CSI_PROVISIONER_NAME` values for that scenario. Add a new test by adding a new
   `e2e-tests/<name>/` directory with its own `_test.go` file and `env` file.

   To iterate on one test directly against an already-running cluster without rerunning all of
   `run_test`:

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
