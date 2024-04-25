package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"k8s.io/klog"

	"github.com/quobyte/quobyte-csi-driver/driver"
)

type DriverRole string

const (
	Node_Driver DriverRole = "node_driver"
	Controller  DriverRole = "controller"
)

var (
	role                         DriverRole
	endpoint                     = flag.String("csi_socket", "unix:///var/lib/kubelet/plugins/quobyte-csi/csi.sock", "CSI endpoint")
	clientMountPoint             = flag.String("quobyte_mount_path", "/mnt/quobyte/mounts", "Mount point for Quobyte Client")
	apiUrlStr                    = flag.String("api_url", "", "Quobyte API URL")
	nodeName                     = flag.String("node_name", "", "Node name from k8s environment")
	driverName                   = flag.String("driver_name", "", "Quobyte CSI driver name")
	driverVersion                = flag.String("driver_version", "", "Quobyte CSI driver version")
	useNameSpaceAsTenant         = flag.Bool("use_k8s_namespace_as_tenant", false, "Uses k8s PVC.namespace as Quobyte tenant")
	enableQuobyteAccessKeyMounts = flag.Bool("enable_access_key_mounts", false, "Enables use of Quobyte Access keys for mounting volumes")
	immediateErase               = flag.Bool("immediate_erase", false, "Schedules erase volume task immediately (supported from Quobyte 3.x)")
	quobyteVersion               = flag.Int("quobyte_version", 2, "Specify Quobyte major version (2 for Quobyte 2.x and 3 for Quobyte 3.x)")
	sharedVolumes                = flag.String("shared_volumes_list", "", "Comma separated list of shared volumes (applicable only for Quobyte 2.x)")
	parallelDeletions            = flag.Int("parallel_deletions", 10, "Delete 'n' shared volume directories parallelly (effective only for Quobyte 2.x)")
)

func main() {
	flag.Func("role", "Driver role (node_driver or controller", func(flagValue string) error {
		if len(flagValue) == 0 {
			return fmt.Errorf("-role is required")
		}
		switch strings.ToLower(flagValue) {
		case "node_driver":
			role = Node_Driver
			return nil
		case "controller":
			role = Controller
			return nil
		default:
			return fmt.Errorf("Unknown role value %s", flagValue)
		}
	})

	klog.InitFlags(nil)
	flag.Parse()
	// logs are available under /tmp/quobyte-csi.* inside quobyte-csi-driver container of the
	// Quobyte CSI Driver pods.
	// We would also need to get the logs of attacher and provisioner pods additionally.

	if *quobyteVersion != 2 && *quobyteVersion != 3 {
		klog.Errorf("--quobyte_version must be 2 for Quobyte 2.x/3 for Quobyte 3.x (given %d)", *quobyteVersion)
		os.Exit(1)
	}

	apiURL, err := url.Parse(*apiUrlStr)
	if err != nil {
		klog.Errorf("Could not parse API '%s' url due to error: %s.", *apiUrlStr, err.Error())
		os.Exit(1)
	}

	// TODO (venkat): remove cleanup of shared volumes with 2.x deprecation
	// This is only required for 2.x and cleanup is only run on active controller
	// and active controller cleans up its own <host_name>_delete_pvc-....> inside shared volume(s) list
	if *quobyteVersion == 2 && role == Controller {
		sharedVols := strings.Split(*sharedVolumes, ",")
		if len(sharedVols) > 0 && *parallelDeletions > 0 {
			klog.Infof("Shared volumes cleanup is enabled for volumes %s", sharedVols)
			// run periodic clean up for shared volumes
			driver.LaunchDirectoryDeleter(*nodeName, *clientMountPoint, sharedVols, *parallelDeletions)
		} else {
			klog.Info("Shared volumes cleanup is turned off")
		}
	}

	qd := driver.NewQuobyteDriver(
		*endpoint,
		*clientMountPoint,
		*nodeName,
		*driverName,
		*driverVersion,
		apiURL,
		*useNameSpaceAsTenant,
		*enableQuobyteAccessKeyMounts,
		*immediateErase,
		*quobyteVersion)
	err = qd.Run()
	if err != nil {
		klog.Errorf("Failed to start Quobyte CSI grpc server due to error: %v.", err)
		os.Exit(1)
	}
	os.Exit(0)
}
