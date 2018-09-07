package driver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"google.golang.org/grpc"
)

const (
	driverName    = "quobyte-csi"
	driverVersion = "0.1"
)

// QuobyteDriver CSI driver type
type QuobyteDriver struct {
	name             string
	version          string
	endpoint         *string
	clientMountPoint string
	server           *grpc.Server
}

// NewQuobyteDriver returns the quobyteDriver object
func NewQuobyteDriver(endpoint, mount *string) *QuobyteDriver {
	return &QuobyteDriver{driverName, driverVersion, endpoint, *mount, nil}
}

// Run starts the grpc server for the driver
func (qd *QuobyteDriver) Run() error {
	u, err := url.Parse(*(qd.endpoint))
	if err != nil {
		glog.Error(err.Error())
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
	glog.Info("Remove socket if it already exists in the path %s", *qd.endpoint)
	if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unix domain socket file %s, error: %v", address, err)
	}

	listener, err := net.Listen(u.Scheme, address)
	if err != nil {
		glog.Errorf("Failed to listen on %s due to error: %v.", address, err)
		return err
	}

	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			glog.Errorf("Method %s failed with error: %v.", info.FullMethod, err)
		} else {
			glog.Infof("Method %s completed.", info.FullMethod)
		}
		return resp, err
	}

	glog.Infof("Starting Quobyte-CSI Driver - driver: '%s' version: '%s' GRPC socket: '%s' mount point: '%s'.", qd.name, qd.version, *qd.endpoint, qd.clientMountPoint)

	qd.server = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterNodeServer(qd.server, qd)
	csi.RegisterControllerServer(qd.server, qd)
	csi.RegisterIdentityServer(qd.server, qd)
	return qd.server.Serve(listener)
}

func (qd *QuobyteDriver) stop() {
	qd.server.Stop()
	glog.Info("CSI driver stopped.")
}
