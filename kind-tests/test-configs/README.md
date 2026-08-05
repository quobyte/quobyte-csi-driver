# Test Configuration

At bare minimum, each test directory contains

* [k8s definitions](#k8s-definitions)
* [CSI driver definition](#csi-driver-definition)

## k8s definitions

Files that start with `k8s_` are deployed by test runner inside kuberentes setup.
At bare minimum, your test should include the following kubernetes resources

* Storage class with reference to your
* `secretes` referred in Storage class. Further these secret data should allow Quobyte API access/
  mounted storage access (with access keys)

These configuration files are deployed in their natural listing order (`ls` output order). If order
is needed, "pseudo" order can be achieved via `k8s_0/a.....`.

## CSI driver definition

This file does NOT start with `k8s_` and should be named as `values.yaml`.
The following values from your `values.yaml` are overriden with
`quobyte.csiProvisionerName: csi.quobyte.com`,
`quobyte.dev.csiProvisionerVersion: <commit-hash>` and
`csi.dev.csiImage: <csi-image-compiled>` during test run.

