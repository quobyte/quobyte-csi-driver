package driver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

// QuobyteDriver CSI driver type
type QuobyteDriver struct {
	Name                            string
	Version                         string
	endpoint                        string
	clientMountPoint                string
	server                          *grpc.Server
	NodeName                        string
	ApiURL                          *url.URL
	UseK8SNamespaceAsQuobyteTenant  bool
	IsQuobyteAccessKeyMountsEnabled bool
	ImmediateErase                  bool
	QuobyteVersion                  int
	quoybteClientFactory QuobyteApiClientProvider
	mounter                        Mounter
}

// NewQuobyteDriver returns the quobyteDriver object
func NewQuobyteDriver(
	endpoint,
	mount,
	nodeName,
	driverName,
	driverVersion string,
	apiURL *url.URL,
	useNamespaceAsQuobyteTenant,
	enableQuobyteAccessKeyMounts bool,
	immediateErase bool,
	quobyteVersion int) *QuobyteDriver {
	return &QuobyteDriver{
		driverName,
		driverVersion,
		endpoint,
		mount,
		nil,
		nodeName,
		apiURL,
		useNamespaceAsQuobyteTenant,
		enableQuobyteAccessKeyMounts,
		immediateErase,
		quobyteVersion,
		&QuobyteApiClientFactory{},
		&LinuxMounter{},
	}
}

// Run starts the grpc server for the driver
func (d *QuobyteDriver) Run() error {
	if len(d.clientMountPoint) == 0 {
		return fmt.Errorf("--quobyte_mount_path is required. Supplied value should match environment variable QUOBYTE_MOUNT_POINT of Quobyte client pod.")
	}
	if len(d.Name) == 0 {
		return fmt.Errorf("--driver_name should not be empty")
	}

	if len(d.Version) == 0 {
		return fmt.Errorf("--driver_version should not be empty")
	}
	u, err := url.Parse(d.endpoint)
	if err != nil {
		klog.Error(err.Error())
	}
	var address string
	if len(u.Host) == 0 {
		address = filepath.FromSlash(u.Path)
	} else {
		address = path.Join(u.Host, filepath.FromSlash(u.Path))
	}
	if u.Scheme != "unix" {
		return fmt.Errorf("CSI currently only supports unix domain sockets, given %s", u.Scheme)
	}
	klog.Infof("Remove socket if it already exists in the path %s", d.endpoint)
	if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unix domain socket file %s, error: %v", address, err)
	}
	listener, err := net.Listen(u.Scheme, address)
	if err != nil {
		klog.Errorf("Failed to listen on %s due to error: %v.", address, err)
		return err
	}
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			klog.Errorf("Method %s failed with error: %v.", info.FullMethod, err)
		} else {
			klog.Infof("Method %s completed.", info.FullMethod)
		}
		return resp, err
	}
	klog.Infof("Starting Quobyte-CSI Driver - driver: '%s' version: '%s'"+
		"GRPC socket: '%s' mount point: '%s' API URL: '%s' "+
		" MapNamespaceNameToQuobyteTenant: %t QuobyteAccesskeysEnabled: %t",
		d.Name, d.Version, d.endpoint, d.clientMountPoint, d.ApiURL,
		d.UseK8SNamespaceAsQuobyteTenant, d.IsQuobyteAccessKeyMountsEnabled)
	d.server = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterNodeServer(d.server, d)
	csi.RegisterControllerServer(d.server, d)
	csi.RegisterIdentityServer(d.server, d)
	return d.server.Serve(listener)
}

func (d *QuobyteDriver) stop() {
	d.server.Stop()
	klog.Info("CSI driver stopped.")
}
