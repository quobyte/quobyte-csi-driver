package driver

import (
	"context"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
)

// GetPluginInfo returns information about plugin
func (d *QuobyteDriver) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	resp := &csi.GetPluginInfoResponse{
		Name:          driverName,
		VendorVersion: driverVersion,
	}
	return resp, nil
}

// GetPluginCapabilities returns plugin capabilities
func (d *QuobyteDriver) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	resp := &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}
	return resp, nil
}

// Probe returns the health and status of the plugin
func (d *QuobyteDriver) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	// TODO: Check if health and status can be determined
	return &csi.ProbeResponse{}, nil
}
