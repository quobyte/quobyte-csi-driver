# Quobyte CSI

Quobyte CSI is the implementation of [Container Storage Interface (CSI)](
    https://github.com/container-storage-interface/spec/tree/release-1.0).
 Quobyte CSI enables easy integration of Quobyte Storage into Kubernetes.

This repository holds source code and development related documentation. For installation
instructions and examples, please refer to
[Quobyte K8S resources](https://github.com/quobyte/quobyte-k8s-resources)


## CSI Driver Options

| Option | Type | Default | Description |
| :--- | :---: | :---: | :--- |
| api_url | string |  | Quobyte API URL |
| driver_name | string |  | Quobyte CSI driver name |
| driver_version | string |  | Quobyte CSI driver version |
| enable_access_key_mounts | boolean | false | Enables use of Quobyte Access keys for mounting volumes |
| enable_volume_metrics | boolean | false | Enables volume metrics (space and inodes) export |
| immediate_erase | boolean | false | Schedules erase volume task immediately |
| node_name | string |  | K8S node name |
| parallel_deletions | int | 10 | Delete 'n' shared volume directories parallelly |
| quobyte_mount_path | string | /mnt/quobyte/mounts | Mount point of Quobyte Client |
| quobyte_version | integer | 4 | Specify Quobyte major version |
| role | string | node_driver/controller | Quobyte CSI container role |
| shared_volumes_list | string |  | Comma separated list of shared volume UUIDs |
| use_delete_files_task | boolean | false | Remove shared volume PVCs using delete files task. If false, uses rmdir via client mount point |
| use_k8s_namespace_as_tenant | boolean | false | Uses k8s PVC.namespace as Quobyte tenant |

## Developer Notes

### Building

Quobyte CSI Driver builds multi-arch (amd64, arm64) images using `docker buildx`. To build images,
containerd storage backed should be enabled for docker.

To **publish Quobyte CSI Driver image, tag release**, run:

```bash
./src/build.sh release <version> # example version: v2.5.2
```

The above command publishes a release image, creates release tag and update remote base.

To **compile in container, run tests, and push only container image**, run:

```bash
./src/build.sh container <version> # example version: v2.5.2
```

The above command publishes a container image (no tags are created).

To **run tests and compile in container**, run:

```bash
./src/build.sh
```

The above command builds a container image but does not publish/load
 (must only be used for sanity check). Further, this does not produce any binary.

To **build binary**:

```
(cd src; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o quobyte-csi ./cmd/main.go)
```

### Testing

To **run tests and compile in container**, run:

```bash
./src/build.sh
```

You can also run

```bash
(cd src; go test -v ./...;)
```

To run **E2E** tests see [here](e2e-tests/README.md)
