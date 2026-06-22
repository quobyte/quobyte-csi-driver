# Quobyte CSI E2e tests

The aim of these set of scripts is to enable CSI e2e test runs against given k8s configuration
and Quobyte setup.

NOTE: These scripts trigger E2E tests. Results needs to manual verified.

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
    kind-tests/cleanup; TEST_CASE_DIR="<absolute-path-to-your-test-case-dir>" kind-tests/run_test
    ```
  
    or

    You can also run `kind-tests/run_test` without `TEST_CASE_DIR` to provision a kubernetes cluster
    . Thereafter, you could `export KUBECONFIG=...` as instructed by script output and install
    csi driver, execute tests manually.

    or

    You can run with `TEST_CASE_DIR` that contains only CSI driver values.yaml to deploy the driver
    (note that some defined values such as CSI image/pod killer images are overridden)

## Cleanup

* To destroy `kind` cluster and other resources, run the following command
  (from project root: quobyte-csi-driver)
  
  ```bash
  kind-tests/cleanup
  ```
