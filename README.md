# Quobyte CSI

Quobyte CSI is the implementation of [Container Storage Interface (CSI)](
    https://github.com/container-storage-interface/spec/tree/release-1.0).
 Quobyte CSI enables easy integration of Quobyte Storage into Kubernetes.

This repository holds source code and development related documentation. For installation
instructions and examples, please refer to
[Quobyte K8S resources](https://github.com/quobyte/quobyte-k8s-resources)

## Developer Notes

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


