package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
)

func TestGetPluginInfo(t *testing.T) {
	d := &QuobyteDriver{}
	csiDriverName := "csi.quobyte.com"
	csiDriverVersion := "v2.2.5"
	d.Name = csiDriverName
	d.Version = csiDriverVersion
	got, err := d.GetPluginInfo(context.TODO(), &csi.GetPluginInfoRequest{})
	assert.Nil(t, err)
	wanted := &csi.GetPluginInfoResponse {
		Name: csiDriverName,
		VendorVersion: csiDriverVersion,
	}
	assert.Equal(t, wanted, got)
}

func TestProbe(t *testing.T) {
	d := &QuobyteDriver{}
	got, err := d.Probe(context.TODO(), &csi.ProbeRequest{})
	assert.Nil(t, err)
	assert.Equal(t, &csi.ProbeResponse{}, got)
}

func TestGetPluginCapabilities(t *testing.T) {
	d := &QuobyteDriver{}
	got, err := d.GetPluginCapabilities(context.TODO(), &csi.GetPluginCapabilitiesRequest{})
	assert.Nil(t, err)
	wanted := &csi.GetPluginCapabilitiesResponse{
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
	assert.Equal(t, wanted, got)
}