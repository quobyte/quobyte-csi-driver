package main

import (
	"flag"
	"os"

	"github.com/golang/glog"

	"github.com/quobyte/quobyte-csi/driver"
)

var (
	endpoint         = flag.String("csi_socket", "unix:///var/lib/kubelet/plugins/quobyte-csi/csi.sock", "CSI endpoint")
	clientMountPoint = flag.String("quobyte_mount_path", "/mnt/quobyte/mounts", "Mount point for Quobyte Client")
	apiURL           = flag.String("api_url", "", "Quobyte API URL")
	nodeName         = flag.String("node_name", "", "Node name from K8S environment")
)

func main() {
	flag.Parse()
	// logs are available under /tmp/quobyte-csi.* inside quobyte-csi-driver plugin pods.
	// We would also need to get the logs of attacher and provisioner pods additionally.

	qd := driver.NewQuobyteDriver(*endpoint, *clientMountPoint, *nodeName, *apiURL)
	err := qd.Run()
	if err != nil {
		glog.Errorf("Failed to start Quobyte CSI grpc server due to eroro: %v.", err)
		os.Exit(1)
	}
	os.Exit(0)
}
