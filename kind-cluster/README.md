# Quobyte CSI E2e tests

The aim of these set of scripts is to enable CSI e2e test runs against internal Quobyte cluster
(testing cluster)

## Requirements

1. Running docker service

2. Installed [kind tool](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).
   Make sure that installed location is part of your $PATH.

3. Installed `helm` tool

4. Installed `kubectl`

5. Quobyte API endpoint and registry endpoint

## Run tests

1. Clone `quobyte-csi` repo & checkout a feature branch

    ```bash
    git clone <https://github.com/quobyte/quobyte-csi-driver.git> && cd quobyte-csi-driver && git checkout <branch/commit>
    ```

2. Setup your test following [test example](./test-configs/)

3. Run your test with command (from project root - quobyte-csi-driver)

    ```bash
    kind-cluster/cleanup; TEST_CASE_DIR="<absolute-path-to-your-test-case-dir>" kind-cluster/run_test
    ```
  
    or

    You can also run `kind-cluster/run_test` without `TEST_CASE_DIR` to provision a kubernetes cluster
    . Thereafter, you could `export KUBECONFIG=...` as instructed by script output and install
    csi driver, execute tests manually.

## Cleanup

* To destroy `kind` cluster and other resources, run the following command
  (from project root: quobyte-csi-driver)
  
  ```bash
  kind-cluster/cleanup
  ```
  