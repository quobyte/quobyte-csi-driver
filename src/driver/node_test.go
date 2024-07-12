package driver

import (
	"context"
	"net/url"
	"strings"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	mock_quobyte_api "github.com/quobyte/api/mocks"
	"github.com/quobyte/api/quobyte"
	"github.com/quobyte/quobyte-csi-driver/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"
)

func TestGetNodeInfo(t *testing.T) {
	nodeName := "some-hostname"
	d := &QuobyteDriver{}
	d.NodeName = nodeName
	got, _ := d.NodeGetInfo(context.TODO(), &csi.NodeGetInfoRequest{})
	wanted := &csi.NodeGetInfoResponse {
		NodeId: nodeName,
	}
	assert.Equal(t, wanted, got);
}

func TestNodePublishVolume(t *testing.T) {
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodePublishVolumeRequest{}
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert := assert.New(t)
	assert.Nil(got)
	assert.NotNil(err)
	assert.True(strings.Contains(err.Error(), "target mount path is empty"))

	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter
	mountPath := "/some/mount/path"
	mounter.EXPECT().CreateMountPath(gomock.Eq(mountPath)).DoAndReturn(
		func(_ string) error {
			return nil
		}).AnyTimes()
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"
	mounter.EXPECT().Mount(gomock.Eq(
		[]string {"-o", "bind", "/quobyte/client/mountpoint/" + volumeUuid,
		mountPath})).Return(nil)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	req.VolumeId = tenantUuid + SEPARATOR + volumeUuid
	got, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse {}
	assert.Equal(wanted, got);

	// Resolve tenant/volume uuid
	tenantName := "some_tenant_name"
	volumeName := "some_volume_name"
	resolvedVolumeUuid := "resolve_volume_uuid"
	quoybteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quoybteClient.EXPECT().GetVolumeUUID(gomock.Eq(volumeName), gomock.Eq(tenantName)).Return(
		resolvedVolumeUuid, nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func (_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quoybteClient, nil
		}).AnyTimes()
	d.quoybteClientFactory = quobyteClientProvider
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	req.VolumeId = tenantName + SEPARATOR + volumeName
	secrets := make(map[string]string)
	secrets[secretUserKey] = "some_management_user"
	secrets[secretPasswordKey] = "some_management_user_password"
	req.Secrets = secrets
	mounter.EXPECT().Mount(gomock.Eq(
		[]string {"-o", "bind", "/quobyte/client/mountpoint/" + resolvedVolumeUuid,
		mountPath})).Return(nil)
	got, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted = &csi.NodePublishVolumeResponse {}
	assert.Equal(wanted, got);

	// TODO(venkat): Expand tests to cover snapshots, mounting with access keys
	// mounting subdirs (with/without snapshots)
}

func TestNodeUnpublishVolume(t *testing.T) {
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodeUnpublishVolumeRequest{}
	got, err := d.NodeUnpublishVolume(context.TODO(), req)
	assert := assert.New(t)
	assert.Nil(got)
	assert.NotNil(err)
	assert.True(strings.Contains(err.Error(), "target path for unmount is empty"))

	unmountPath := "/some/mounted/path"
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	mounter.EXPECT().Unmount(gomock.Eq(unmountPath))
	d.mounter = mounter
	req = &csi.NodeUnpublishVolumeRequest{}
	req.TargetPath = unmountPath
	got, _ = d.NodeUnpublishVolume(context.TODO(), req)
	wanted := &csi.NodeUnpublishVolumeResponse {}
	assert.Equal(wanted, got);
}

func TestNodeGetVolumeStats(t *testing.T) {
	driverName := "my.quobyte.csi.provisioner"
	d := &QuobyteDriver{}
	d.Name = driverName
	d.enabledVolumeMetrics = false
	got, err := d.NodeGetVolumeStats(context.TODO(), &csi.NodeGetVolumeStatsRequest{})
	assert := assert.New(t)
	assert.Nil(got);
	assert.NotNil(err);
	assert.Contains(err.Error(), "disabled for the Quobyte CSI Driver " + driverName)
	d = &QuobyteDriver{}
	req := &csi.NodeGetVolumeStatsRequest{}
	d.enabledVolumeMetrics = true
	got, err = d.NodeGetVolumeStats(context.TODO(), req)
	assert.Nil(got)
	assert.NotNil(err)
	assert.True(strings.Contains(err.Error(), "volume path must not be empty"))

	mountedPath := "/some/mounted/path"
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	mounter.EXPECT().Statfs(gomock.Eq(mountedPath)).DoAndReturn(
		func(_ string) (unix.Statfs_t, error) {
			return unix.Statfs_t{
				Bsize: 2,
				Blocks: 10,
				Bfree: 5,
				Bavail: 3,
				Files: 20,
				Ffree: 10,
			}, nil
		})
	d.mounter = mounter
	req = &csi.NodeGetVolumeStatsRequest{}
	req.VolumePath = mountedPath
	got, _ = d.NodeGetVolumeStats(context.TODO(), req)
	wanted := &csi.NodeGetVolumeStatsResponse {
		Usage: []*csi.VolumeUsage {
			{
				Unit:      csi.VolumeUsage_BYTES,
				Total: 20, // Blocks * Bsize
				Used: 10, // (Blocks - Bfree) * Bsize
				Available: 6, // Bavail * Bsize
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Total: 20,
				Used: 10,
				Available: 10,
			},
		},
	}
	assert.Equal(wanted, got);
}
