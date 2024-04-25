package driver

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	mock_quobyte_api "github.com/quobyte/api/mocks"
	"github.com/quobyte/api/quobyte"
	"github.com/quobyte/quobyte-csi-driver/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreateVolume(t *testing.T) {
	d := &QuobyteDriver{}
	req := &csi.CreateVolumeRequest{}
	got, err := d.CreateVolume(context.TODO(), req)
	assert := assert.New(t)
	assert.NotNil(err)
	assert.Nil(got)
}

func TestDeleteVolume(t *testing.T) {
	d := &QuobyteDriver{}
	req := &csi.DeleteVolumeRequest{}
	got, err := d.DeleteVolume(context.TODO(), req)
	assert := assert.New(t)
	assert.NotNil(err)
	assert.Nil(got)
	assert.True(strings.Contains(err.Error(), "volumeId is required for DeleteVolume"))
	volume := "some-volume"
	tenant := "some-tenant"
	req = &csi.DeleteVolumeRequest{
		VolumeId: volume,
	}
	got, err = d.DeleteVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Nil(got)
	assert.True(strings.Contains(err.Error(), "is not in the form <Tenant_Name/Tenant_UUID>"))

	req = &csi.DeleteVolumeRequest{
		VolumeId: tenant + SEPARATOR + volume,
	}
	got, err = d.DeleteVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Nil(got)
	assert.True(strings.Contains(err.Error(), "secrets are required to delete a volume."))

	req = &csi.DeleteVolumeRequest{
		VolumeId: tenant + SEPARATOR + volume,
	}
	got, err = d.DeleteVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Nil(got)
	assert.True(strings.Contains(err.Error(), "secrets are required to delete a volume."))

	ctrl := gomock.NewController(t)
	quoybteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quoybteClient.EXPECT().EraseVolumeByResolvingNamesToUUID(
		gomock.Eq(volume), gomock.Eq(tenant), gomock.Any()).Return(nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func (_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quoybteClient, nil
		}).AnyTimes()
	d.quoybteClientFactory = quobyteClientProvider
	secrets := make(map[string]string)
	secrets[secretUserKey] = "some_management_user"
	secrets[secretPasswordKey] = "some_management_user_password"
	req = &csi.DeleteVolumeRequest{
		VolumeId: tenant + SEPARATOR + volume,
		Secrets: secrets,
	}
	got, err = d.DeleteVolume(context.TODO(), req)
	assert.Nil(err)
	assert.NotNil(got)
	assert.Equal(&csi.DeleteVolumeResponse{}, got)
	// TODO(venkat): add snapshots and subdir (shared volume) tests
}

func TestControllerGetCapabilities(t *testing.T) {
	d := &QuobyteDriver{}
	resp, err := d.ControllerGetCapabilities(context.TODO(), &csi.ControllerGetCapabilitiesRequest{})
	assert := assert.New(t)
	assert.Nil(err)
	assert.NotNil(resp)
	gotCapabilities := parseCapabilities(resp.Capabilities)
	wanted := []csi.ControllerServiceCapability_RPC_Type {
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	}
	assert.Equal(wanted, gotCapabilities)
}

func parseCapabilities(capabilities []*csi.ControllerServiceCapability) ([]csi.ControllerServiceCapability_RPC_Type) {
	var caps []csi.ControllerServiceCapability_RPC_Type
	for _, capability := range capabilities {
		caps = append(caps, capability.GetRpc().Type)
	}
	return caps
}