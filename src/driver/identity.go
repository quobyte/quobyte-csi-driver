package driver

import (
	"context"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

// GetPluginInfo returns information about driver
func (d *QuobyteDriver) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	resp := &csi.GetPluginInfoResponse{
		Name:          d.Name,
		VendorVersion: d.Version,
	}
	return resp, nil
}

// GetPluginCapabilities returns driver capabilities
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
			{
				Type: &csi.PluginCapability_VolumeExpansion_{
					VolumeExpansion: &csi.PluginCapability_VolumeExpansion{
						Type: csi.PluginCapability_VolumeExpansion_ONLINE,
					},
				},
			},
		},
	}
	return resp, nil
}

// Probe returns the health and status of the driver
func (d *QuobyteDriver) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	// TODO: Check if health and status can be determined
	return &csi.ProbeResponse{}, nil
}
