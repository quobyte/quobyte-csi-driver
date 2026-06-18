package driver

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	mock_quobyte_api "github.com/quobyte/api/v4/mocks"
	"github.com/quobyte/api/v4/quobyte"
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
	wanted := &csi.NodeGetInfoResponse{
		NodeId: nodeName,
	}
	assert := assert.New(t)
	assert.Equal(wanted, got)
}

func TestNodePublishVolumeMountErrorHandling(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter

	req := &csi.NodePublishVolumeRequest{}
	req.TargetPath = "somePodPath"
	req.VolumeId = "tenant|volume"
	mounter.EXPECT().Mkdirs(gomock.Any(), gomock.Any()).Return(nil)
	mounter.EXPECT().Mount(gomock.Any(), gomock.Any()).Return(fmt.Errorf("mount failed"))

	response, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "mount failed")
}

func TestNodePublishVolumeInvalidMountOptionsHandling(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter

	req := &csi.NodePublishVolumeRequest{}
	req.TargetPath = "somePodPath"
	req.VolumeId = "tenant|volume"
	req.VolumeCapability = &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{
				MountFlags: []string{"ro"},
			}}}
	mounter.EXPECT().Mkdirs(gomock.Any(), gomock.Any()).Return(nil)

	response, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Mount options are not supported by Quobyte CSI Driver")
}

func TestNodePublishVolumeQuobyteCreationErrorHandling(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter

	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return nil, fmt.Errorf("client cannot be created")
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	mounter.EXPECT().Mkdirs(gomock.Any(), gomock.Any()).Return(nil)

	req := &csi.NodePublishVolumeRequest{}
	req.TargetPath = "somePodPath"
	req.VolumeId = "tenant|volume"
	// User/password is not sufficient for access keys
	req.Secrets = map[string]string{
		secretUserKey:     "someUser",
		secretPasswordKey: "somePassword",
	}
	response, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "client cannot be created")
}

func TestNodePublishVolumeInvalidAccessKeySecretsHandling(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter
	d.IsQuobyteAccessKeyMountsEnabled = true

	req := &csi.NodePublishVolumeRequest{}
	req.TargetPath = "somePodPath"
	req.VolumeId = "tenant|volume"
	mounter.EXPECT().Mkdirs(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mounter.EXPECT().CreateAccessKeyContextHandle().Return("someAccessKeyContextHandle").AnyTimes()

	response, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Mount secret should have")

	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(gomock.Any(), gomock.Any()).Return(
		"someVolumeUuid", nil).AnyTimes()
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = "somePodPath"
	req.VolumeId = "tenant|volume"
	// User/password is not sufficient for access keys
	req.Secrets = map[string]string{
		secretUserKey:     "someUser",
		secretPasswordKey: "somePassword",
	}
	response, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Mount secret should have")

	req.Secrets = map[string]string{
		accessKeyID:       "someAccessKey",
		secretPasswordKey: "somePassword",
	}
	response, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Mount secret should have")

	req.Secrets = map[string]string{
		accessKeySecret:   "someAccessKeySecret",
		secretPasswordKey: "somePassword",
	}
	response, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Mount secret should have")

	mounter.EXPECT().SetXAttr(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		fmt.Errorf("some xattr set error"))
	req.Secrets = map[string]string{
		accessKeyID:     "someAccessKeyId",
		accessKeySecret: "someAccessKeySecret",
	}
	response, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "some xattr set error")
}

func TestNodePublishVolumeSanity(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter

	// Missing target pod mount
	req := &csi.NodePublishVolumeRequest{}
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(got)
	assert.NotNil(err)
	assert.Contains(err.Error(), "given target mount path is empty")

	// Cannot create pod requested path to mount
	req.TargetPath = "somePodPath"
	firstCreateCall := mounter.EXPECT().Mkdirs(gomock.Any(), gomock.Any()).Return(
		fmt.Errorf("some create path error"))
	response, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "create path error")

	mounter.EXPECT().Mkdirs(gomock.Any(), gomock.Any()).Return(nil).After(firstCreateCall).AnyTimes()
	// Invalid Quobyte volume handle to mount - snapshot must have volume context with snapshot ID
	req.VolumeId = SnapshotVolumeHandlePrefix + "someSnapshotHandle"
	response, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "volume context should not empty for snapshot")

	req.VolumeId = "invalidVolumeHandle" // valid handle should be at least <tenant>|<volume>
	response, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "is not in the format")

	// Simulate volume uuid resolution failure
	req.VolumeId = "tenant|volume"
	req.Secrets = map[string]string{
		secretUserKey:     "someUser",
		secretPasswordKey: "somePassword",
	}
	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(gomock.Any(), gomock.Any()).Return(
		"", fmt.Errorf("volume resolution failed"))
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	response, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(response)
	assert.NotNil(err)
	assert.Contains(err.Error(), "volume resolution failed")
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
	mounter.EXPECT().Mkdirs(gomock.Eq(mountPath), gomock.Any()).DoAndReturn(
		func(_ string, _ os.FileMode) error {
			return nil
		}).AnyTimes()
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+volumeUuid,
		mountPath).Return(nil)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	req.VolumeId = tenantUuid + VOLUME_HANDLE_PART_SEPARATOR + volumeUuid
	got, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)

	// Resolve tenant/volume uuid
	tenantName := "some_tenant_name"
	volumeName := "some_volume_name"
	resolvedVolumeUuid := "resolve_volume_uuid"
	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(gomock.Eq(volumeName), gomock.Eq(tenantName)).Return(
		resolvedVolumeUuid, nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	req.VolumeId = tenantName + VOLUME_HANDLE_PART_SEPARATOR + volumeName
	secrets := make(map[string]string)
	secrets[secretUserKey] = "some_management_user"
	secrets[secretPasswordKey] = "some_management_user_password"
	req.Secrets = secrets
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+resolvedVolumeUuid,
		mountPath).Return(nil)
	got, err = d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted = &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodePublishVolumeSubdir(t *testing.T) {
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodePublishVolumeRequest{}

	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter
	mountPath := "/some/mount/path"
	mounter.EXPECT().Mkdirs(gomock.Eq(mountPath), gomock.Any()).DoAndReturn(
		func(_ string, _ os.FileMode) error {
			return nil
		}).AnyTimes()
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"
	subDir := "some_sub_dir"
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+volumeUuid+"/"+subDir,
		mountPath).Return(nil)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	req.VolumeId = tenantUuid + VOLUME_HANDLE_PART_SEPARATOR + volumeUuid + VOLUME_HANDLE_PART_SEPARATOR + subDir
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodePublishVolumeAccessKeyMount(t *testing.T) {
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodePublishVolumeRequest{}

	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.IsQuobyteAccessKeyMountsEnabled = true
	d.mounter = mounter
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"
	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(gomock.Any(), gomock.Any()).Return(
		volumeUuid, nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	mountPath := "/some/mount/path"
	mountAccessKeyId := "some_access_key_id"
	mountAccessKeySecret := "some_access_key_secret"
	mountAccessKeySecrets := map[string]string{
		accessKeyID:     mountAccessKeyId,
		accessKeySecret: mountAccessKeySecret,
	}
	accessKeyContextHandle := uuid.New().String()
	mounter.EXPECT().Mkdirs(gomock.Eq(mountPath), gomock.Any()).DoAndReturn(
		func(_ string, _ os.FileMode) error {
			return nil
		}).AnyTimes()
	mounter.EXPECT().SetXAttr(
		xAttrKey,
		getAccessKeyValStr(
			mountAccessKeyId,
			mountAccessKeySecret,
			accessKeyContextHandle),
		d.clientMountPoint).Return(nil)
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+accessKeyContextHandle+
		accessKeyContextHandleSeparator+volumeUuid,
		mountPath).Return(nil)
	mounter.EXPECT().CreateAccessKeyContextHandle().Return(accessKeyContextHandle)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	req.VolumeId = tenantUuid + VOLUME_HANDLE_PART_SEPARATOR + volumeUuid
	req.Secrets = mountAccessKeySecrets
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodePublishVolumeSubdirAccessKeyMount(t *testing.T) {
	d := &QuobyteDriver{}
	d.IsQuobyteAccessKeyMountsEnabled = true
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodePublishVolumeRequest{}

	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"

	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(gomock.Any(), gomock.Any()).Return(
		volumeUuid, nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	mountPath := "/some/mount/path"
	mountAccessKeyId := "some_access_key_id"
	mountAccessKeySecret := "some_access_key_secret"
	mountAccessKeySecrets := map[string]string{
		accessKeyID:     mountAccessKeyId,
		accessKeySecret: mountAccessKeySecret,
	}
	accessKeyContextHandle := uuid.New().String()
	mounter.EXPECT().SetXAttr(
		xAttrKey,
		getAccessKeyValStr(
			mountAccessKeyId,
			mountAccessKeySecret,
			accessKeyContextHandle),
		d.clientMountPoint).Return(nil)
	mounter.EXPECT().Mkdirs(gomock.Eq(mountPath), gomock.Any()).DoAndReturn(
		func(_ string, _ os.FileMode) error {
			return nil
		}).AnyTimes()

	subDir := "some_sub_dir"
	mounter.EXPECT().CreateAccessKeyContextHandle().Return(accessKeyContextHandle)
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+accessKeyContextHandle+
		accessKeyContextHandleSeparator+volumeUuid+"/"+subDir,
		mountPath).Return(nil)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	req.VolumeId = tenantUuid + VOLUME_HANDLE_PART_SEPARATOR + volumeUuid + VOLUME_HANDLE_PART_SEPARATOR + subDir
	req.Secrets = mountAccessKeySecrets
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodePublishVolumeSnapshotAccessKeyMount(t *testing.T) {
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodePublishVolumeRequest{}

	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.IsQuobyteAccessKeyMountsEnabled = true
	d.mounter = mounter
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"
	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(gomock.Any(), gomock.Any()).Return(
		volumeUuid, nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	mountPath := "/some/mount/path"
	mountAccessKeyId := "some_access_key_id"
	mountAccessKeySecret := "some_access_key_secret"
	mountAccessKeySecrets := map[string]string{
		accessKeyID:     mountAccessKeyId,
		accessKeySecret: mountAccessKeySecret,
	}
	accessKeyContextHandle := uuid.New().String()
	mounter.EXPECT().Mkdirs(gomock.Eq(mountPath), gomock.Any()).DoAndReturn(
		func(_ string, _ os.FileMode) error {
			return nil
		}).AnyTimes()
	mounter.EXPECT().SetXAttr(
		xAttrKey,
		getAccessKeyValStr(
			mountAccessKeyId,
			mountAccessKeySecret,
			accessKeyContextHandle),
		d.clientMountPoint).Return(nil)
	snapshotName := "someSnapshot"
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+accessKeyContextHandle+
		accessKeyContextHandleSeparator+volumeUuid+"/"+snapshotsDir+"/"+snapshotName,
		mountPath).Return(nil)
	mounter.EXPECT().CreateAccessKeyContextHandle().Return(accessKeyContextHandle)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	// For snapshots, we do not create a Quobyte volume but create a marker PV with snapshot prefix
	req.VolumeId = SnapshotVolumeHandlePrefix + "dummy"
	snapshotHandle := tenantUuid + VOLUME_HANDLE_PART_SEPARATOR + volumeUuid + VOLUME_HANDLE_PART_SEPARATOR + snapshotName
	req.VolumeContext = map[string]string{
		SnapshotIDKey: snapshotHandle,
	}
	req.Secrets = mountAccessKeySecrets
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodePublishSnapshotSubdirAccessKeyMount(t *testing.T) {
	d := &QuobyteDriver{}
	d.IsQuobyteAccessKeyMountsEnabled = true
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodePublishVolumeRequest{}

	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"

	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(gomock.Any(), gomock.Any()).Return(
		volumeUuid, nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	mountPath := "/some/mount/path"
	mountAccessKeyId := "some_access_key_id"
	mountAccessKeySecret := "some_access_key_secret"
	mountAccessKeySecrets := map[string]string{
		accessKeyID:     mountAccessKeyId,
		accessKeySecret: mountAccessKeySecret,
	}
	accessKeyContextHandle := uuid.New().String()
	mounter.EXPECT().SetXAttr(
		xAttrKey,
		getAccessKeyValStr(
			mountAccessKeyId,
			mountAccessKeySecret,
			accessKeyContextHandle),
		d.clientMountPoint).Return(nil)
	mounter.EXPECT().Mkdirs(gomock.Eq(mountPath), gomock.Any()).DoAndReturn(
		func(_ string, _ os.FileMode) error {
			return nil
		}).AnyTimes()

	snapshotName := "someSnapshot"
	subDir := "some_sub_dir"
	mounter.EXPECT().CreateAccessKeyContextHandle().Return(accessKeyContextHandle)
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+accessKeyContextHandle+accessKeyContextHandleSeparator+volumeUuid+"/"+snapshotsDir+
		"/"+snapshotName+"/"+subDir,
		mountPath).Return(nil)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	// For snapshots, we do not create a Quobyte volume but create a marker PV with snapshot prefix
	req.VolumeId = SnapshotVolumeHandlePrefix + "dummy"
	snapshotHandle := tenantUuid + VOLUME_HANDLE_PART_SEPARATOR + volumeUuid + VOLUME_HANDLE_PART_SEPARATOR + snapshotName + VOLUME_HANDLE_PART_SEPARATOR + subDir
	req.VolumeContext = map[string]string{
		SnapshotIDKey: snapshotHandle,
	}
	req.Secrets = mountAccessKeySecrets
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodePublishSnapshotVolume(t *testing.T) {
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodePublishVolumeRequest{}

	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter
	mountPath := "/some/mount/path"
	mounter.EXPECT().Mkdirs(gomock.Eq(mountPath), gomock.Any()).DoAndReturn(
		func(_ string, _ os.FileMode) error {
			return nil
		}).AnyTimes()
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"
	snapshotName := "someSnapshot"
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+volumeUuid+"/"+snapshotsDir+"/"+snapshotName,
		mountPath).Return(nil)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	// For snapshots, we do not create a Quobyte volume but create a marker PV with snapshot prefix
	req.VolumeId = SnapshotVolumeHandlePrefix + "dummy"
	snapshotHandle := tenantUuid + VOLUME_HANDLE_PART_SEPARATOR + volumeUuid + VOLUME_HANDLE_PART_SEPARATOR + snapshotName
	req.VolumeContext = map[string]string{
		SnapshotIDKey: snapshotHandle,
	}
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodePublishSnapshotVolumeSubdir(t *testing.T) {
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath
	req := &csi.NodePublishVolumeRequest{}

	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	d.mounter = mounter
	mountPath := "/some/mount/path"
	mounter.EXPECT().Mkdirs(gomock.Eq(mountPath), gomock.Any()).DoAndReturn(
		func(_ string, _ os.FileMode) error {
			return nil
		}).AnyTimes()
	tenantUuid := "some_tenant_uuid"
	volumeUuid := "some_volume_uuid"
	snapshotName := "someSnapshot"
	subDir := "some_sub_dir"
	mounter.EXPECT().Mount("/quobyte/client/mountpoint/"+volumeUuid+"/"+snapshotsDir+
		"/"+snapshotName+"/"+subDir,
		mountPath).Return(nil)
	req = &csi.NodePublishVolumeRequest{}
	req.TargetPath = mountPath
	// For snapshots, we do not create a Quobyte volume but create a marker PV with snapshot prefix
	req.VolumeId = SnapshotVolumeHandlePrefix + "dummy"
	snapshotHandle := tenantUuid + VOLUME_HANDLE_PART_SEPARATOR + volumeUuid + VOLUME_HANDLE_PART_SEPARATOR + snapshotName + VOLUME_HANDLE_PART_SEPARATOR + subDir
	req.VolumeContext = map[string]string{
		SnapshotIDKey: snapshotHandle,
	}
	got, err := d.NodePublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodePublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodeUnpublishVolume(t *testing.T) {
	d := &QuobyteDriver{}
	clientMountPath := "/quobyte/client/mountpoint"
	d.clientMountPoint = clientMountPath

	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	req := &csi.NodeUnpublishVolumeRequest{}
	got, err := d.NodeUnpublishVolume(context.TODO(), req)
	assert.Nil(got)
	assert.NotNil(err)
	assert.True(strings.Contains(err.Error(), "target path for unmount is empty"))

	mounter.EXPECT().Unmount(gomock.Any()).Return(fmt.Errorf("some unmount error"))
	req = &csi.NodeUnpublishVolumeRequest{}
	req.TargetPath = "someUnmountPath"
	d.mounter = mounter
	_, err = d.NodeUnpublishVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.True(strings.Contains(err.Error(), "some unmount error"))

	unmountPath := "/some/mounted/path"
	mounter.EXPECT().Unmount(gomock.Eq(unmountPath)).Return(nil)
	req = &csi.NodeUnpublishVolumeRequest{}
	req.TargetPath = unmountPath
	got, err = d.NodeUnpublishVolume(context.TODO(), req)
	assert.Nil(err)
	wanted := &csi.NodeUnpublishVolumeResponse{}
	assert.Equal(wanted, got)
}

func TestNodeGetCapabilities(t *testing.T) {
	d := &QuobyteDriver{}
	got, err := d.NodeGetCapabilities(context.TODO(), &csi.NodeGetCapabilitiesRequest{})
	assert := assert.New(t)
	assert.Nil(err)
	wanted := &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
		},
	}
	assert.Equal(wanted, got)
}

func TestNodeStageVolume(t *testing.T) {
	d := &QuobyteDriver{}
	assert := assert.New(t)
	got, err := d.NodeStageVolume(context.TODO(), &csi.NodeStageVolumeRequest{})
	assert.Nil(got)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Not implemented")
}

func TestProcessVolumeHandle(t *testing.T) {
	assert := assert.New(t)
	_, _, _, err := processVolumeHandle("wrongHandle")
	assert.NotNil(err)
	tenant, volume, subdir, err := processVolumeHandle("tenant|volume")
	assert.Nil(err)
	assert.Equal(tenant, "tenant")
	assert.Equal(volume, "volume")
	assert.Equal(subdir, "")
	tenant, volume, subdir, err = processVolumeHandle("tenant|volume|subDir")
	assert.Equal(tenant, "tenant")
	assert.Equal(volume, "volume")
	assert.Equal(subdir, "subDir")
}

func TestGetMountOptions(t *testing.T) {
	assert := assert.New(t)
	req := &csi.NodePublishVolumeRequest{}
	mountOpts := getMountOptions(req)
	assert.Nil(mountOpts)
	req.VolumeCapability = &csi.VolumeCapability{}
	mountOpts = getMountOptions(req)
	req.VolumeCapability = &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{
				MountFlags: nil,
			}}}
	mountOpts = getMountOptions(req)
	req.VolumeCapability = &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{
				MountFlags: []string{"ro"},
			}}}
	mountOpts = getMountOptions(req)
	assert.NotNil(mountOpts)
}

func TestMayGetVolumeHandleFromSnapshotContext(t *testing.T) {
	assert := assert.New(t)
	req := &csi.NodePublishVolumeRequest{
		VolumeId: "tenant|volume", // Non-snapshot volume handle/id
	}
	volumeHandle, snapshotName, err := mayGetVolumeHandleFromSnapshotContext(req)
	assert.Nil(err)
	assert.Equal(req.VolumeId, volumeHandle)
	assert.Equal("", snapshotName)

	req = &csi.NodePublishVolumeRequest{
		VolumeId: SnapshotVolumeHandlePrefix + "some-k8s-snapshot-identifier",
	}
	volumeHandle, snapshotName, err = mayGetVolumeHandleFromSnapshotContext(req)
	assert.NotNil(err)
	assert.Contains(err.Error(), "volume context should not empty for snapshot")
	assert.Equal("", volumeHandle)
	assert.Equal("", snapshotName)

	req = &csi.NodePublishVolumeRequest{
		VolumeId:      SnapshotVolumeHandlePrefix + "some-k8s-snapshot-identifier",
		VolumeContext: map[string]string{},
	}
	volumeHandle, snapshotName, err = mayGetVolumeHandleFromSnapshotContext(req)
	assert.NotNil(err)
	assert.Contains(err.Error(), SnapshotIDKey+" key is not found in the volume context")
	assert.Equal("", volumeHandle)
	assert.Equal("", snapshotName)

	req = &csi.NodePublishVolumeRequest{
		VolumeId: SnapshotVolumeHandlePrefix + "some-k8s-snapshot-identifier",
		VolumeContext: map[string]string{
			SnapshotIDKey: "tenant|volume"},
	}
	volumeHandle, snapshotName, err = mayGetVolumeHandleFromSnapshotContext(req)
	assert.NotNil(err)
	assert.Contains(err.Error(),
		getInvalidSnapshotIdError(req.GetVolumeContext()[SnapshotIDKey]).Error())
	assert.Equal("", volumeHandle)
	assert.Equal("", snapshotName)

	req = &csi.NodePublishVolumeRequest{
		VolumeId: SnapshotVolumeHandlePrefix + "some-k8s-snapshot-identifier",
		VolumeContext: map[string]string{
			SnapshotIDKey: "tenant|volume|snapshot"},
	}
	volumeHandle, snapshotName, err = mayGetVolumeHandleFromSnapshotContext(req)
	assert.Nil(err)
	assert.Equal("tenant|volume", volumeHandle)
	assert.Equal("snapshot", snapshotName)

	req = &csi.NodePublishVolumeRequest{
		VolumeId: SnapshotVolumeHandlePrefix + "some-k8s-snapshot-identifier",
		VolumeContext: map[string]string{
			SnapshotIDKey: "tenant|volume|snapshot|subdir"},
	}
	volumeHandle, snapshotName, err = mayGetVolumeHandleFromSnapshotContext(req)
	assert.Nil(err)
	assert.Equal("tenant|volume|subdir", volumeHandle)
	assert.Equal("snapshot", snapshotName)
}

func TestNodeUnstageVolume(t *testing.T) {
	d := &QuobyteDriver{}
	assert := assert.New(t)
	got, err := d.NodeUnstageVolume(context.TODO(), &csi.NodeUnstageVolumeRequest{})
	assert.Nil(got)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Not implemented")
}

func TestNodeExpandVolume(t *testing.T) {
	d := &QuobyteDriver{}
	assert := assert.New(t)
	got, err := d.NodeExpandVolume(context.TODO(), &csi.NodeExpandVolumeRequest{})
	assert.Nil(got)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Not implemented")
}

func TestNodeGetVolumeStats(t *testing.T) {
	driverName := "my.quobyte.csi.provisioner"
	d := &QuobyteDriver{}
	d.Name = driverName
	d.enabledVolumeMetrics = false
	got, err := d.NodeGetVolumeStats(context.TODO(), &csi.NodeGetVolumeStatsRequest{})
	assert := assert.New(t)
	assert.Nil(got)
	assert.NotNil(err)
	assert.Contains(err.Error(), "disabled for the Quobyte CSI Driver "+driverName)
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
	mounter.EXPECT().Statfs(gomock.Eq(mountedPath)).Return(unix.Statfs_t{}, fmt.Errorf("some error"))
	d.mounter = mounter
	req = &csi.NodeGetVolumeStatsRequest{}
	req.VolumePath = mountedPath
	_, err = d.NodeGetVolumeStats(context.TODO(), req)
	assert.NotNil(err)

	mounter.EXPECT().Statfs(gomock.Eq(mountedPath)).DoAndReturn(
		func(_ string) (unix.Statfs_t, error) {
			return unix.Statfs_t{
				Bsize:  2,
				Blocks: 10,
				Bfree:  5,
				Bavail: 3,
				Files:  20,
				Ffree:  10,
			}, nil
		})

	req = &csi.NodeGetVolumeStatsRequest{}
	req.VolumePath = mountedPath
	got, _ = d.NodeGetVolumeStats(context.TODO(), req)
	wanted := &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Total:     20, // Blocks * Bsize
				Used:      10, // (Blocks - Bfree) * Bsize
				Available: 6,  // Bavail * Bsize
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Total:     20,
				Used:      10,
				Available: 10,
			},
		},
	}
	assert.Equal(wanted, got)
}
