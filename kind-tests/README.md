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
- **Go e2e suite** (`RUN_GO_E2E_TESTS=true`): runs the self-contained suite in
  [`e2e-tests/`](./e2e-tests), which builds its own Secret/StorageClass/PVC/Pod in Go (uniquely
  named per run), writes/reads a file through the pod's Quobyte mount, and talks directly to the
  Quobyte API to confirm the backing volume was actually created -- so results are asserted by
  `go test`, not eyeballed.

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

3. To run the Go e2e suite instead of the legacy flow, set `RUN_GO_E2E_TESTS=true` and provide
   `QUOBYTE_API_URL`, `QUOBYTE_API_USER`, `QUOBYTE_API_PASSWORD`, and `QUOBYTE_TENANT` in the
   environment (same pattern as `TEST_CASE_DIR`/`CSI_PROVISIONER_NAME` -- `run_test` just forwards
   them to `go test`, it does not read them from any YAML file):

    ```bash
    RUN_GO_E2E_TESTS=true \
    QUOBYTE_API_URL=http://<host>:<port> \
    QUOBYTE_API_USER=<user> QUOBYTE_API_PASSWORD=<password> QUOBYTE_TENANT="My Tenant" \
    TEST_CASE_DIR="<absolute-path-to-your-test-case-dir>" kind-tests/run_test
    ```

   To iterate on the Go tests against an already-running cluster without rerunning all of
   `run_test`:

    ```bash
    cd kind-tests/e2e-tests
    KUBECONFIG=/tmp/quobyte-k8s-config NAMESPACE=quobyte \
    QUOBYTE_API_URL=http://<host>:<port> \
    QUOBYTE_API_USER=<user> QUOBYTE_API_PASSWORD=<password> QUOBYTE_TENANT="My Tenant" \
    CSI_PROVISIONER_NAME=csi.quobyte.com \
    go test ./... -v -run TestDynamicProvisioning -timeout 20m
    ```

## Cleanup

* To destroy `kind` cluster and other resources, run the following command
  (from project root: quobyte-csi-driver)
  
  ```bash
  kind-tests/cleanup
  ```
