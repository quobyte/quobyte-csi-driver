# Quobyte CSI E2e tests

The aim of these set of scripts is to enable CSI e2e test runs against given k8s configuration
and Quobyte setup.

`test_runner` provisions the kind cluster, builds the CSI driver and pod killer from source (or,
with `USE_CHART_IMAGES=true`, deploys the images already named in the chart's `values.yaml`
instead), and then runs **two sets** of self-contained Go test packages against it,
redeploying the Quobyte client and CSI driver for each one:

| Test set | What a test does |
| --- | --- |
| [`e2e-sanity-tests/`](./e2e-sanity-tests) | Quobyte's own checks of the driver. |
| [`e2e-upstream-tests/`](./e2e-upstream-tests) | Sets up what an upstream Kubernetes suite needs via the Quobyte API, runs the suite, then removes that setup again. |

Both are laid out the same way: a test package carries an `env` **directory**, every file
in it is one driver setup, and the package is run once per file.

Both sets share the [`framework/`](./framework) package and one Go module rooted at
`kind-tests/` (`github.com/quobyte/quobyte-csi-driver/kind-tests`).

## How a test is run

For every (test package, environment file) combination found, `test_runner`:

1. **Deploys the environment** -- exports the variables of that `env` file (see
   [Environment files](#environment-files)).
2. Deploys the Quobyte client via the [`quobyte-client`](./quobyte-k8s-resources/helm/quobyte-client)
   helm chart using `QUOBYTE_REGISTRY`/`ENABLE_ACCESS_KEY_MOUNTS`, and the CSI driver (built from
   source) via the [`quobyte-csi`](./quobyte-k8s-resources/helm/quobyte-csi) helm chart using
   that environment's values file and `--set` overrides.
3. Creates a randomized namespace for the test's own resources and **runs the test** --
   `go test ./<name>/...` for just that package.
4. Tears down the namespace and both helm releases.
5. **Removes the environment** -- unsets that `env` file's variables, so nothing leaks into the
   next environment, and moves on to the next combination.

A test package with several environment files therefore goes through the whole deploy/run/undeploy
cycle once per file, each time against a freshly deployed driver.

### Running several at once

`PARALLEL_TESTS` (default 5) is how many combinations run at the same time. Each one needs a kind
cluster to itself -- the driver and client are deployed under fixed helm release names, and the
client is a DaemonSet owning a mount point per node -- so a worker means a cluster of its own, four
containers and a driver deployment each. That is the number to weigh when raising it. The
combinations are handed out round robin, so a worker that draws the long upstream suites may still
be going when the others are done.

They all share one Quobyte installation, which is why each test creates its own tenant, its own
Quobyte user and its own Kubernetes namespace, all named uniquely per run
(`framework.CreateTestUser`).

A worker deletes its cluster when it has finished the combinations it was given, so a green run
leaves nothing behind. Two exceptions: the cluster of a test that failed is always left up (that
is the one to debug -- its kubeconfig is printed with the failure, and `test-env.txt` in the debug
directory names both), and `KEEP_CLUSTERS=true` keeps every cluster, which is what the
[iterate-against-a-running-cluster](#run-tests) workflow below needs.

`PARALLEL_TESTS=1` is the sequential run: one cluster, keeping the name and kubeconfig path used
below, and the only mode that streams test output to the terminal. With more workers the terminal
shows progress lines only and everything else is collected in the debug directory:

```
kind-csi-testing/debug/clusters/<cluster>.log       bringing that worker's cluster up
kind-csi-testing/debug/<test>/<env file>/run.log    one combination, deploy to undeploy
kind-csi-testing/debug/<test>/<env file>/           the failure snapshot described below
```

Every run ends with a summary of each combination, named `<test package>/<env file>`:

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

A combination's duration is its whole deploy/run/undeploy cycle, not just `go test`, so it includes
the client and driver helm installs done for it -- but not the one-off creation of its worker's
cluster, which is reported separately as the worker starts. `NOT RUN` combinations show `-`, as
does the one a stopped run was in the middle of. With several workers the durations overlap, so
they do not add up to the elapsed time.

Those names are also how you select what to run. `--sanity` / `--upstream` narrow the run to one
test set, `TESTS` takes a space-separated list of combination names with glob patterns included,
and `test_runner --list` prints the available ones without touching anything:

```bash
kind-tests/test_runner --list
kind-tests/test_runner --sanity --list      # just the sanity set

# only the sanity tests, skipping the long upstream suites
kind-tests/test_runner --sanity http://host:port host:port

# re-run just the one that failed
TESTS='dynamic_provisioning/default' kind-tests/test_runner http://host:port host:port

# or every environment of one test package, by naming the package
TESTS='external_storage' kind-tests/test_runner http://host:port host:port

# several patterns at once; globs still work
TESTS='expansion volume_metrics shared_volume*/default' kind-tests/test_runner http://host:port host:port
```

A `TESTS` entry matches a combination three ways: the whole name, the package part of one
(`external_storage`, with or without a trailing `/`, meaning every environment of it), or a glob
pattern. The package form stops at the directory boundary, so `shared_volume` runs the two
environments of `shared_volume/` and leaves `shared_volume_cleanup/` alone — use a glob
(`shared_volume*`) if you do mean both.

The selection is resolved before the kind cluster is built, so a name that matches nothing fails
immediately instead of after the cluster is up.

On failure `test_runner` stops without cleaning up: that worker's cluster, the driver, the client and
the test's own resources are all left running for live debugging (the Go tests use
`framework.CleanupUnlessFailed`, which skips their own teardown when the test failed). The other
workers finish the combination they are on and then stop taking new work, so what is left is
reported as `NOT RUN`. Set `CONTINUE_ON_FAILURE=true` to run the whole matrix instead and get a
complete table -- the failed combination is then torn down like any other, so only its debug
snapshot survives. Either way `test_runner` exits non-zero if anything failed.

For every failed combination a best-effort debug snapshot -- pods, events, driver and client
logs, plus a copy of the Secret/StorageClass the test applied -- is written to
`kind-csi-testing/debug/<test>/<env file>/`.

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
| `CSI_VALUES_FILE` | no | A values file for the `quobyte-csi` chart, absolute or relative to the test directory. Defaults to the chart's own `values.yaml`. |
| `ENABLE_SNAPSHOTS` | no | Passed to the upstream suite; snapshot tests additionally need the driver deployed with `quobyte.enableSnapshots=true`. |
| `USE_K8S_NAMESPACE_AS_TENANT` | no | Tests read it to know that a StorageClass leaving `quobyteTenant` unset is provisioned into the tenant named after the PVC's namespace. The driver additionally needs `quobyte.useK8SNamespaceAsTenant=true` in `CSI_HELM_SET`; keep the two in step. |
| `USE_SEPARATE_MOUNT_SECRET` | no | Split the driver's two uses of a Secret across two: a Quobyte management access key for provisioning/expansion, and a separate data access key that the node-publish secret carries for mounting. Requires `ENABLE_ACCESS_KEY_MOUNTS`. |
| `USE_SHARED_VOLUME` | no | Provision through a Quobyte shared volume: the test adds a `sharedVolumeName` parameter to its StorageClass, so every PVC becomes a subdirectory of that one volume instead of a volume of its own. The test names the volume itself, uniquely per run. |
| `PRE_CREATE_SHARED_VOLUME` | no | With `USE_SHARED_VOLUME`, have the test create that volume through the Quobyte API during setup (`framework.CreateSharedVolume`) and delete it again afterwards, instead of leaving the driver to create it on the first provisioning request. Setting it without `USE_SHARED_VOLUME` fails the test. |
| `QUOBYTE_TENANT` | no | Pin a pre-existing tenant instead of the unique per-run name `test_runner` generates. |
| `GO_TEST_TIMEOUT` | no | `-timeout` for this test's `go test` run (`test_runner` default: `20m`). |

`quobyte.dev.csiImage`/`quobyte.dev.podKillerImage`/`quobyte.dev.csiProvisionerVersion` are
overridden by `test_runner` with the locally built images, whichever values file is used --
unless `USE_CHART_IMAGES=true` (`test_runner --help`), in which case they are left exactly as
the values file has them and nothing is built from source.

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
backing volume was actually created -- so results are asserted by `go test`, not eyeballed.

The credentials in that Secret belong to a Quobyte user each test creates for itself
(`framework.CreateTestUser`), admin of that run's tenant and nothing else, exactly as the upstream
test does. `QUOBYTE_API_USER`/`QUOBYTE_API_PASSWORD` are used only to set the run up -- creating
tenants, users and access keys -- and are never handed to the driver: shared across tests, that
user's tenant mappings go stale as tests delete the tenants they created, and the next test to
update them is refused with "tenant not found".

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
   is needed.

## Run tests

Run from the project root (`quobyte-csi-driver`):

```bash
kind-tests/cleanup
kind-tests/test_runner http://host:port host:port
# or, overriding the admin/quobyte user/password defaults:
kind-tests/test_runner http://host:port host:port myuser mypassword
```

`test_runner` requires the Quobyte API endpoint and client registry as its first two
arguments (see Requirements above) -- it `die`s immediately with a usage message if
either is missing. It also requires a clean git working tree, so commit your changes
before running it.

To iterate on one test directly against an already-running cluster without rerunning all of
`test_runner` (cluster creation, image build, etc.). The cluster has to still be there, so either the
run that made it failed, or it was told to keep it:

```bash
KEEP_CLUSTERS=true PARALLEL_TESTS=1 TESTS='dynamic_provisioning/default' \
  kind-tests/test_runner http://host:port host:port
```

then, against `kind-tests/tmp/kubeconfig-cluster` (worker 1's cluster; further workers add a
`-<worker>` suffix to both the cluster name and this path):

```bash
cd kind-tests/e2e-sanity-tests
set -a; source dynamic_provisioning/env/default; set +a
KUBECONFIG=kind-tests/tmp/kubeconfig-cluster NAMESPACE=quobyte QUOBYTE_TENANT=my-tenant \
QUOBYTE_API_URL=http://host:port QUOBYTE_API_USER=admin QUOBYTE_API_PASSWORD=secret \
CSI_PROVISIONER_NAME=csi.quobyte.com \
go test ./dynamic_provisioning/... -v -timeout 20m
```

The same works for the upstream tests, which additionally need to know where the suite script
and the checkout are:

```bash
cd kind-tests/e2e-upstream-tests
set -a; source external_storage/env/default; set +a
KUBECONFIG=kind-tests/tmp/kubeconfig-cluster NAMESPACE=quobyte QUOBYTE_TENANT=my-tenant \
QUOBYTE_API_URL=http://host:port QUOBYTE_API_USER=admin QUOBYTE_API_PASSWORD=secret \
CSI_PROVISIONER_NAME=csi.quobyte.com ARTIFACTS_DIR=/tmp/e2e-artifacts \
UPSTREAM_E2E_SCRIPT="$(git rev-parse --show-toplevel)/kind-tests/e2e" \
REPO_ROOT="$(git rev-parse --show-toplevel)" \
go test ./external_storage/... -v -timeout "$GO_TEST_TIMEOUT"
```

## Cleanup

* To destroy the `kind` clusters and other resources, run the following command
  (from project root: quobyte-csi-driver)

  ```bash
  kind-tests/cleanup
  ```

  A run normally deletes its own clusters as the workers finish; this removes whatever is
  left -- the cluster of a failed test, the clusters of a run that was interrupted or that
  used `KEEP_CLUSTERS=true` -- along with their kubeconfigs, the kind node image and
  `kind-csi-testing/`, and with it the debug output of that run. It only ever touches
  clusters named `quobyte-csi-testing` and `quobyte-csi-testing-<worker>`.
