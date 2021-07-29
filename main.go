package main

import (
	"flag"
	"net/url"
	"os"

	"k8s.io/klog"

	"github.com/quobyte/quobyte-csi/driver"
)

var (
	endpoint               = flag.String("csi_socket", "unix:///var/lib/kubelet/plugins/quobyte-csi/csi.sock", "CSI endpoint")
	clientMountPoint       = flag.String("quobyte_mount_path", "/mnt/quobyte/mounts", "Mount point for Quobyte Client")
	apiUrlStr              = flag.String("api_url", "", "Quobyte API URL")
	nodeName               = flag.String("node_name", "", "Node name from k8s environment")
	driverName             = flag.String("driver_name", "", "Quobyte CSI driver name")
	driverVersion          = flag.String("driver_version", "", "Quobyte CSI driver version")
	useNameSpaceAsTenant   = flag.Bool("use_k8s_namespace_as_tenant", false, "Uses k8s PVC.namespace as Quobyte tenant")
	enableQuobyteAcceskeys = flag.Bool("enable_access_keys", false, "Enables use of Quobyte Access keys for mounting volumes")
)

func main() {
	flag.Set("alsologtostderr", "true")
	klog.InitFlags(nil)
	flag.Parse()
	// logs are available under /tmp/quobyte-csi.* inside quobyte-csi-driver plugin pods.
	// We would also need to get the logs of attacher and provisioner pods additionally.

	apiURL, err := url.Parse(*apiUrlStr)
	if err != nil {
		klog.Errorf("Could not parse API '%s' url due to eroro: %s.", *apiUrlStr, err.Error())
		os.Exit(1)
	}

	qd := driver.NewQuobyteDriver(
		*endpoint,
		*clientMountPoint,
		*nodeName,
		*driverName,
		*driverVersion,
		apiURL,
		*useNameSpaceAsTenant,
		*enableQuobyteAcceskeys)
	err = qd.Run()
	if err != nil {
		klog.Errorf("Failed to start Quobyte CSI grpc server due to eroro: %v.", err)
		os.Exit(1)
	}
	os.Exit(0)
}
