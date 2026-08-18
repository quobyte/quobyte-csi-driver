# Quobyte CSI E2e tests

The aim of these set of scripts is to enable CSI e2e test runs against given k8s configuration
and Quobyte setup.

`test_runner` provisions the kind cluster, sources the quobyte-csi/quobyte-client charts and
images per `--source` (see [Chart/image sources](#chartimage-sources) below), and then runs
**two sets** of self-contained Go test packages against the provisioned cluster redeploying the
Quobyte client and CSI driver for each test:

| Test set | What a test does |
| --- | --- |
| [`e2e-sanity-tests/`](./e2e-sanity-tests) | Quobyte's own checks of the driver. |
| [`e2e-upstream-tests/`](./e2e-upstream-tests) | Sets up what an upstream Kubernetes suite needs via the Quobyte API, runs the suite, then removes that setup again. |

Both set of tests are organized in the same way: a test package carries an `env` **directory**, every file
in it is one driver setup, and the package is run once per the "env/" file.

Both set of tests share the [`framework/`](./framework) package and one Go module rooted at
`kind-tests/` (`github.com/quobyte/quobyte-csi-driver/kind-tests`).

## Requirements

1. Running docker service

2. Installed [kind tool](https://kind.sigs.k8s.io/docs/user/quick-start/#installation).
   Make sure that installed location is part of your $PATH.

3. Installed `helm` tool

4. Installed `kubectl`

5. Installed `go`

6. Quobyte API endpoint and registry endpoint, passed to `test_runner` as positional
   arguments: `<QUOBYTE_API_URL> <QUOBYTE_REGISTRY> [QUOBYTE_API_USER] [QUOBYTE_API_PASSWORD]`.
   The URL and registry are required; user/password are optional and default to
   `admin`/`quobyte` if omitted. `CSI_PROVISIONER_NAME` also defaults to `csi.quobyte.com`
   inside `test_runner`; override by pre-setting the env var if a different provisioner name
   is needed. (`QUOBYTE_API_USER/QUOBYTE_API_PASSWORD` are used to create tenants and associated
   tenant users during tests)

## How a test is run

For every [test package, environment file] combination, the `test_runner` repeats the following:

1. **Deploys the environment** -- exports the variables of that `env` file (see
   [Environment files](#environment-files)).
2. Deploys the Quobyte client via the [`quobyte-client`](./quobyte-k8s-resources/helm/quobyte-client)
   helm chart using `QUOBYTE_REGISTRY`/`ENABLE_ACCESS_KEY_MOUNTS`, and the CSI driver via
   the [`quobyte-csi`](./quobyte-k8s-resources/helm/quobyte-csi) with "env/" as helm
   `--set` overrides.
3. Creates a randomized namespace for the test's own resources and **runs the test** --
   `go test ./<name>/...` for just that package.
4. Tears down the namespace and both helm releases.
5. **Removes the environment** -- unsets that `env` file's variables, so nothing leaks into the
   next environment, and moves on to the next combination.

A test package with several environment files therefore goes through the whole deploy/run/undeploy
cycle once per file, each time against a freshly deployed driver.

## Chart/image sources

`--source` (default `build`) picks where the quobyte-csi/quobyte-client charts and the images
they deploy come from:

| `--source` | Charts come from | Images | Requires |
| --- | --- | --- | --- |
| `build` (default) | the `quobyte-k8s-resources` submodule's chart directories | built from this checkout and `kind load`ed | clean working tree; both submodules checked out |
| `chart-images` | the `quobyte-k8s-resources` submodule's chart directories | whatever `quobyte.dev.csiImage`/`podKillerImage` the chart's own `values.yaml` (or a test's `CSI_VALUES_FILE`/`CSI_HELM_SET` override) names, pulled normally | clean working tree; `quobyte-k8s-resources` submodule checked out |
| `oci-charts` | `oci://quay.io/quobyte/charts/quobyte-csi` and `.../quobyte-client`, at `QUOBYTE_CSI_CHART_VERSION`/`QUOBYTE_CLIENT_CHART_VERSION` | whatever those released charts' own `values.yaml` names | `QUOBYTE_CSI_CHART_VERSION` and `QUOBYTE_CLIENT_CHART_VERSION` set; network access to `quay.io` |

`build` is the only mode that compiles the CSI driver/pod killer from this checkout; the other
two deploy released images as-is. `oci-charts` is the only mode that never touches either
submodule -- it `helm pull`s the `quobyte-csi` chart once (to read its default `values.yaml`
and the snapshot CRD/controller manifests, which are not Helm templates) and installs both
charts straight from their `oci://` references.

With `oci-charts`, a debug dump of a failed test collects driver logs with plain `kubectl logs`
instead of the submodule's `log_collector.sh`, which the published chart does not include.

## Run tests

Run from the project root (`quobyte-csi-driver`):

```bash
kind-tests/test_runner --list  # list all tests
kind-tests/test_runner --sanity --list      # just list the sanity set
kind-tests/test_runner --upstream --list      # just list the upstream test set

# Run all the tests
kind-tests/test_runner <http://host:port> <host:port> <client-image-url>
# Run only the sanity tests, skipping the long upstream suites
kind-tests/test_runner --sanity <http://host:port> <host:port> <client-image-url>
# Run only the upstream suites
kind-tests/test_runner --sanity <http://host:port> <host:port> <client-image-url>

# Deploy the quobyte-k8s-resources submodule's charts as they are (nothing built locally)
kind-tests/test_runner --source=chart-images <http://host:port> <host:port> <client-image-url>

# Deploy released charts straight from quay.io (no submodule, nothing built locally)
QUOBYTE_CSI_CHART_VERSION=1.8.14 QUOBYTE_CLIENT_CHART_VERSION=0.3.4 \
  kind-tests/test_runner --source=oci-charts <http://host:port> <host:port> <client-image-url>
```

Only select tests can be run with `TESTS` (takes a space-separated list of combination names).
For example

```bash
kind-tests/test_runner --list  # List all the tests

# re-run just the one that failed
TESTS='dynamic_provisioning/default' kind-tests/test_runner <http://host:port> <host:port> <client-image-url>

# or every environment of one test package, by naming the package
TESTS='external_storage' kind-tests/test_runner <http://host:port> <host:port> <client-image-url>

# several patterns at once; globs still work
TESTS='expansion volume_metrics shared_volume*/default' kind-tests/test_runner <http://host:port> <host:port> <client-image-url>
```

After each run, `cleanup` using:

```bash
kind-tests/cleanup
```

## Results

Every run ends with a summary of each combination, named `<test package>/<env file>`:

For example:

```
==== Test summary ====
TEST                              RESULT   DURATION
dynamic_provisioning/access_keys  PASS     8m12s
dynamic_provisioning/default      PASS     7m48s
external_storage/access_keys      NOT RUN  -
external_storage/default          FAIL     21m05s

2 passed, 1 failed, 1 not run
1h15m30s elapsed, of which 18m20s building the images
Output of every test is under kind-csi-testing/debug/<test>/<environment>/
```

### Running several at once

`PARALLEL_TESTS` (default 3) is how many combinations run at the same time. Each parallel run
creates its own kind cluster (4 k8s node containers per cluster). Each cluster runs
a test as outlined [above](#how-a-test-is-run). Tests are selected in round robin fashion by each
cluster.

They all share one Quobyte installation but each test creates its own tenant, its own
Quobyte user and its own Kubernetes namespace, all named uniquely per run.

A worker deletes its cluster when it has finished the combinations it was given, so a green run
leaves nothing behind. If test fails, see [debug](#run-tests) section.

`PARALLEL_TESTS=1` is the sequential run - one k8s cluster is created and one test is run at a time.

## Environment files

An `env` file holds only what varies per driver deployment. The Quobyte API endpoint and client
registry (`QUOBYTE_API_URL`/`QUOBYTE_API_USER`/`QUOBYTE_API_PASSWORD`/`QUOBYTE_REGISTRY`) and
`CSI_PROVISIONER_NAME` are *not* part of it -- they are script-level config set by `test_runner`
itself (see [Requirements](#requirements)), since they are the same physical endpoints and driver
for every test in a run.

| Variable | Required | Meaning |
| --- | --- | --- |
| `ENABLE_ACCESS_KEY_MOUNTS` | yes | Deploy the Quobyte client with access key contexts. Tests read it to decide whether their Secret carries `user`/`password` or `accessKeyId`/`accessKeySecret`. |
| `CSI_HELM_SET` | no | Space-separated `key=value` pairs appended as `--set` to the `quobyte-csi` helm install, e.g. `"quobyte.enableAccessKeyMounts=true"`. |
| `ENABLE_SNAPSHOTS` | no | Passed to the upstream suite; snapshot tests additionally need the driver deployed with `quobyte.enableSnapshots=true`. |
| `USE_K8S_NAMESPACE_AS_TENANT` | no | Tests read this and setup storage class accordingly. The driver additionally needs `quobyte.useK8SNamespaceAsTenant=true` in `CSI_HELM_SET` |
| `USE_SEPARATE_MOUNT_SECRET` | no | Uses different access keys for management API and file system access. Requires `ENABLE_ACCESS_KEY_MOUNTS`. |
| `USE_SHARED_VOLUME` | no | Provision PVC in a shared Quobyte volume |
| `PRE_CREATE_SHARED_VOLUME` | no | Used with `USE_SHARED_VOLUME` flag, have the test create that volume through the Quobyte API during setup (`framework.CreateSharedVolume`) and delete it again afterwards, instead of leaving the driver to create it on the first provisioning request. Setting it without `USE_SHARED_VOLUME` fails the test. |
| `QUOBYTE_TENANT` | no | Pin a pre-existing tenant instead of the unique per-run name `test_runner` generates. |
| `GO_TEST_TIMEOUT` | no | `-timeout` for this test's `go test` run (`test_runner` default: `20m`). |

## The sanity tests

Each subdirectory of [`e2e-sanity-tests/`](./e2e-sanity-tests) with an `env/` directory is one
test package:

```
e2e-sanity-tests/dynamic_provisioning/
  dynamic_provisioning_test.go
  env/
    default        # ENABLE_ACCESS_KEY_MOUNTS=false
    access_keys    # ENABLE_ACCESS_KEY_MOUNTS=true + quobyte.enableAccessKeyMounts=true
```

These tests build their own Secret/StorageClass/PVC/Pod in Go (uniquely named per run), write and
read a file through the pod's Quobyte mount, and talk directly to the Quobyte API to confirm the
backing volume was actually created. This ensures that results are asserted by `go test`.

Add a scenario either by adding a file to an existing `env/` directory (same test, another driver
setup) or by adding a new `e2e-sanity-tests/<name>/` directory with its own `env/` and `_test.go`
files.

## The upstream tests

Each subdirectory of [`e2e-upstream-tests/`](./e2e-upstream-tests) with an `env/` directory is one
test package that drives an upstream Kubernetes suite, and is run once per environment just like a
sanity test:

```
e2e-upstream-tests/external_storage/
  external_storage_test.go
  env/
    default        # user/password mounts, no snapshots
    access_keys    # ENABLE_ACCESS_KEY_MOUNTS=true + quobyte.enableAccessKeyMounts=true
```

The suite itself is not ours -- `kind-tests/e2e` downloads the released `e2e.test`/`ginkgo`
binaries matching the cluster version and runs the sig-storage "external storage" tests -- and it
only knows how to create PVCs from a StorageClass. So the test brackets it:

1. **setup**: create the tenant and a dedicated Quobyte user for this run through the Quobyte Go
   API (`framework.EnsureTenant`, `framework.CreateUser`), then the k8s Secret holding that user's
   credentials -- an access key of that user (`framework.CreateAccessKey`) where the environment
   mounts with access keys, or two Secrets with a management and a data access key where it asks
   for a separate mount secret.
2. **run**: write the StorageClass to `$ARTIFACTS_DIR` and hand it to `kind-tests/e2e` via
   `framework.RunUpstreamE2E`, which passes it to `e2e.test` as `StorageClass: FromFile`. Ginkgo's
   output is streamed into the test's output.
3. **cleanup**: delete the Secret, the access key, the user and the tenant again. The suite cleans
   up its own namespaces, PVCs and StorageClass copies.


## Cleanup

* To destroy the `kind` clusters and other resources, run the following command
  (from project root: quobyte-csi-driver)

  ```bash
  kind-tests/cleanup
  ```
