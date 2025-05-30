module github.com/quobyte/quobyte-csi-driver

go 1.22.2
toolchain go1.24.1

require (
	github.com/container-storage-interface/spec v1.9.0
	github.com/golang/protobuf v1.5.4
	github.com/google/uuid v1.6.0
	github.com/hashicorp/golang-lru v1.0.2
	github.com/quobyte/api v1.4.0
	github.com/stretchr/testify v1.9.0
	go.uber.org/mock v0.4.0
	golang.org/x/sys v0.31.0
	google.golang.org/grpc v1.63.2
	k8s.io/klog v1.0.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240227224415-6ceb2ff114de // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Uncomment only during testing with local version of Quobyte API
// replace github.com/quobyte/api => /home/venkat/go/api
