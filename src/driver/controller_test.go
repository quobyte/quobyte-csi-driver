package driver

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	mock_quobyte_api "github.com/quobyte/api/v4/mocks"
	"github.com/quobyte/api/v4/quobyte"
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

	ctrl := gomock.NewController(t)
	clientFactory := mocks.NewMockQuobyteApiClientProvider(ctrl)
	client := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	clientFactory.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).Return(client, nil).AnyTimes()
	d = &QuobyteDriver{
		quobyteClientFactory: clientFactory,
	}

	pvName := "pv-abcdef"
	storageClassTenant := "storageClassTenant"
	user := "user"
	primaryGroup := "group"
	client.EXPECT().WhoAmI(gomock.Any()).Return(&quobyte.WhoAmIResponse{
		UserName: user, PrimaryGroup: primaryGroup}, nil).AnyTimes()
	client.EXPECT().GetTenantUUID(gomock.Any()).Return("storageClassTenant", nil).AnyTimes()
	client.EXPECT().CreateVolume(gomock.Any()).DoAndReturn(
		func(req *quobyte.CreateVolumeRequest) (*quobyte.CreateVolumeResponse, error) {
			assert.Equal(pvName, req.Name)
			assert.Equal(storageClassTenant, req.TenantId)
			assert.Equal(user, req.RootUserId)
			assert.Equal(primaryGroup, req.RootGroupId)
			return &quobyte.CreateVolumeResponse{VolumeUuid: "created-volume-uuid"}, nil
		})
	req = &csi.CreateVolumeRequest{
		Name: pvName,
		Parameters: map[string]string{
			"quobyteTenant": storageClassTenant,
		},
		Secrets:       map[string]string{"a": "b"},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
	}
	resp, err := d.CreateVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal(storageClassTenant+"|created-volume-uuid", resp.Volume.VolumeId)

	userOverride := "storage-class-user"
	groupOverride := "storage-class-group"
	client.EXPECT().CreateVolume(gomock.Any()).DoAndReturn(
		func(req *quobyte.CreateVolumeRequest) (*quobyte.CreateVolumeResponse, error) {
			assert.Equal(pvName, req.Name)
			assert.Equal(storageClassTenant, req.TenantId)
			assert.Equal(userOverride, req.RootUserId)
			assert.Equal(groupOverride, req.RootGroupId)
			assert.Equal([]*quobyte.Label{
				{
					Name:  "k",
					Value: "v",
				},
				{
					Name:  "k1",
					Value: "v1",
				},
			}, req.Label)
			return &quobyte.CreateVolumeResponse{VolumeUuid: "created-volume-uuid"}, nil
		})
	req = &csi.CreateVolumeRequest{
		Name: pvName,
		Parameters: map[string]string{
			"quobyteTenant": storageClassTenant,
			"user":          userOverride,
			"group":         groupOverride,
			"accessMode":    "750",
			"labels":        "k:v, k1:v1",
		},
		Secrets:       map[string]string{"a": "b"},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
	}
	resp, err = d.CreateVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal(storageClassTenant+"|created-volume-uuid", resp.Volume.VolumeId)
}

func TestCreateVolumeWithQuota(t *testing.T) {
	d := &QuobyteDriver{}
	req := &csi.CreateVolumeRequest{}
	got, err := d.CreateVolume(context.TODO(), req)
	assert := assert.New(t)
	assert.NotNil(err)
	assert.Nil(got)

	ctrl := gomock.NewController(t)
	clientFactory := mocks.NewMockQuobyteApiClientProvider(ctrl)
	client := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	clientFactory.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).Return(client, nil).AnyTimes()
	d = &QuobyteDriver{
		quobyteClientFactory: clientFactory,
	}

	pvName := "pv-abcdef"
	storageClassTenant := "storageClassTenant"
	user := "user"
	primaryGroup := "group"
	client.EXPECT().WhoAmI(gomock.Any()).Return(&quobyte.WhoAmIResponse{
		UserName: user, PrimaryGroup: primaryGroup}, nil).AnyTimes()
	client.EXPECT().GetTenantUUID(gomock.Any()).Return("storageClassTenant", nil).AnyTimes()
	client.EXPECT().CreateVolume(gomock.Any()).DoAndReturn(
		func(req *quobyte.CreateVolumeRequest) (*quobyte.CreateVolumeResponse, error) {
			assert.Equal(pvName, req.Name)
			assert.Equal(storageClassTenant, req.TenantId)
			assert.Equal(user, req.RootUserId)
			assert.Equal(primaryGroup, req.RootGroupId)
			return &quobyte.CreateVolumeResponse{VolumeUuid: "created-volume-uuid"}, nil
		}).AnyTimes()
	gomock.InOrder(
		client.EXPECT().SetVolumeQuota(gomock.Any(), gomock.Any()).DoAndReturn(
			func(volumeUuid string, size int64) error {
				assert.Equal(size, int64(1000))
				assert.Equal("created-volume-uuid", volumeUuid)
				return nil
			}),
		client.EXPECT().SetVolumeQuota(gomock.Any(), gomock.Any()).DoAndReturn(
			func(volumeUuid string, size int64) error {
				assert.Equal(size, int64(1000))
				assert.Equal("created-volume-uuid", volumeUuid)
				return errors.New("some set quota error")
			}))
	req = &csi.CreateVolumeRequest{
		Name: pvName,
		Parameters: map[string]string{
			"quobyteTenant": storageClassTenant,
			"createQuota":   "true",
		},
		Secrets:       map[string]string{"a": "b"},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
	}
	resp, err := d.CreateVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal(storageClassTenant+"|created-volume-uuid", resp.Volume.VolumeId)

	// Setting Quota failure should trigger created volume deletion

	client.EXPECT().DeleteVolumeByResolvingNamesToUUID(
		gomock.Any(), gomock.Any()).Return(nil)
	req = &csi.CreateVolumeRequest{
		Name: pvName,
		Parameters: map[string]string{
			"quobyteTenant": storageClassTenant,
			"createQuota":   "true",
		},
		Secrets:       map[string]string{"a": "b"},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
	}
	resp, err = d.CreateVolume(t.Context(), req)
	assert.NotNil(err)
	assert.Nil(resp)
}

func TestCreateVolumeIdempotency(t *testing.T) {
	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	clientFactory := mocks.NewMockQuobyteApiClientProvider(ctrl)
	client := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	clientFactory.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).Return(client, nil).AnyTimes()
	d := &QuobyteDriver{
		quobyteClientFactory: clientFactory,
	}

	pvName := "pv-abcdef"
	storageClassTenant := "storageClassTenant"
	user := "user"
	primaryGroup := "group"
	client.EXPECT().WhoAmI(gomock.Any()).Return(&quobyte.WhoAmIResponse{
		UserName: user, PrimaryGroup: primaryGroup}, nil).AnyTimes()
	client.EXPECT().GetTenantUUID(gomock.Any()).Return("storageClassTenant", nil).AnyTimes()
	client.EXPECT().CreateVolume(gomock.Any()).DoAndReturn(
		func(req *quobyte.CreateVolumeRequest) (*quobyte.CreateVolumeResponse, error) {
			assert.Equal(pvName, req.Name)
			assert.Equal(storageClassTenant, req.TenantId)
			assert.Equal(user, req.RootUserId)
			assert.Equal(primaryGroup, req.RootGroupId)
			return nil, errors.New("ENTITY_EXISTS_ALREADY/POSIX_ERROR_NONE")
		})
	client.EXPECT().ResolveVolumeNameToUUID(gomock.Any(), gomock.Any()).Return(
		"existing-volume-uuid",
		nil)
	req := &csi.CreateVolumeRequest{
		Name: pvName,
		Parameters: map[string]string{
			"quobyteTenant": storageClassTenant,
		},
		Secrets:       map[string]string{"a": "b"},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
	}
	resp, err := d.CreateVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal(storageClassTenant+"|existing-volume-uuid", resp.Volume.VolumeId)
}

func TestCreateVolumeWithNamespaceAsTenant(t *testing.T) {
	assert := assert.New(t)

	ctrl := gomock.NewController(t)
	clientFactory := mocks.NewMockQuobyteApiClientProvider(ctrl)
	client := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	clientFactory.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).Return(client, nil).AnyTimes()
	d := &QuobyteDriver{
		UseK8SNamespaceAsQuobyteTenant: true,
		quobyteClientFactory:           clientFactory,
	}
	pvName := "pv-abcdef"
	storageClassTenant := "storageClassTenant"
	user := "user"
	primaryGroup := "group"
	client.EXPECT().WhoAmI(gomock.Any()).Return(&quobyte.WhoAmIResponse{
		UserName: user, PrimaryGroup: primaryGroup}, nil).AnyTimes()

	req := &csi.CreateVolumeRequest{
		Name:          pvName,
		Parameters:    map[string]string{},
		Secrets:       map[string]string{"a": "b"},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
	}
	resp, err := d.CreateVolume(t.Context(), req)
	assert.NotNil(err)
	assert.Contains(err.Error(), "should be deployed with --extra-create-metadata=true")

	namespaceTenant := "pvc-namespace"
	client.EXPECT().GetTenantUUID(gomock.Eq(namespaceTenant)).Return(namespaceTenant, nil)
	client.EXPECT().CreateVolume(gomock.Any()).DoAndReturn(
		func(req *quobyte.CreateVolumeRequest) (*quobyte.CreateVolumeResponse, error) {
			assert.Equal(pvName, req.Name)
			assert.Equal(namespaceTenant, req.TenantId)
			assert.Equal(user, req.RootUserId)
			assert.Equal(primaryGroup, req.RootGroupId)
			return &quobyte.CreateVolumeResponse{VolumeUuid: "created-volume-uuid"}, nil
		})
	req.Parameters[pvcNamespaceKey] = namespaceTenant
	resp, err = d.CreateVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal(namespaceTenant+"|created-volume-uuid", resp.Volume.VolumeId)

	// override driver level namespace tenant setting with storage class (resource) level setting
	req.Parameters["quobyteTenant"] = storageClassTenant
	client.EXPECT().GetTenantUUID(gomock.Any()).Return("storageClassTenant", nil)
	client.EXPECT().CreateVolume(gomock.Any()).DoAndReturn(
		func(req *quobyte.CreateVolumeRequest) (*quobyte.CreateVolumeResponse, error) {
			assert.Equal(pvName, req.Name)
			assert.Equal(storageClassTenant, req.TenantId)
			assert.Equal(user, req.RootUserId)
			assert.Equal(primaryGroup, req.RootGroupId)
			return &quobyte.CreateVolumeResponse{VolumeUuid: "created-volume-uuid"}, nil
		})
	resp, err = d.CreateVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal(storageClassTenant+"|created-volume-uuid", resp.Volume.VolumeId)
}

func TestCreateVolumeWithNoTenant(t *testing.T) {
	assert := assert.New(t)

	ctrl := gomock.NewController(t)
	clientFactory := mocks.NewMockQuobyteApiClientProvider(ctrl)
	client := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	clientFactory.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).Return(client, nil).AnyTimes()
	d := &QuobyteDriver{
		UseK8SNamespaceAsQuobyteTenant: false,
		quobyteClientFactory:           clientFactory,
	}
	pvName := "pv-abcdef"
	user := "user"
	primaryGroup := "group"
	client.EXPECT().WhoAmI(gomock.Any()).Return(&quobyte.WhoAmIResponse{
		UserName: user, PrimaryGroup: primaryGroup}, nil).AnyTimes()

	req := &csi.CreateVolumeRequest{
		Name:          pvName,
		Parameters:    map[string]string{},
		Secrets:       map[string]string{"a": "b"},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
	}

	client.EXPECT().CreateVolume(gomock.Any()).DoAndReturn(
		func(req *quobyte.CreateVolumeRequest) (*quobyte.CreateVolumeResponse, error) {
			assert.Equal(pvName, req.Name)
			assert.Equal(0, len(req.TenantId))
			assert.Equal(0, len(req.TenantDomain))
			assert.Equal(user, req.RootUserId)
			assert.Equal(primaryGroup, req.RootGroupId)
			return &quobyte.CreateVolumeResponse{VolumeUuid: "created-volume-uuid"}, nil
		})
	resp, err := d.CreateVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal("|created-volume-uuid", resp.Volume.VolumeId)
}

type ExistingDirMock struct {
	os.FileInfo
	name string
}

func (m *ExistingDirMock) IsDir() bool {
	return true
}

func TestCreateInSharedVolume(t *testing.T) {
	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	clientFactory := mocks.NewMockQuobyteApiClientProvider(ctrl)
	client := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	clientFactory.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).Return(client, nil).AnyTimes()
	d := &QuobyteDriver{
		quobyteClientFactory: clientFactory,
		mounter:              mounter,
	}

	pvName := "pv-abcdef"
	storageClassTenant := "storageClassTenant"
	user := "user"
	primaryGroup := "group"
	sharedVolumeName := "sharedVolume"
	client.EXPECT().WhoAmI(gomock.Any()).Return(&quobyte.WhoAmIResponse{
		UserName: user, PrimaryGroup: primaryGroup}, nil).AnyTimes()
	client.EXPECT().GetTenantUUID(gomock.Any()).Return("storageClassTenant", nil).AnyTimes()

	client.EXPECT().CreateVolume(gomock.Any()).DoAndReturn(
		func(req *quobyte.CreateVolumeRequest) (*quobyte.CreateVolumeResponse, error) {
			assert.Equal(sharedVolumeName, req.Name)
			assert.Equal(storageClassTenant, req.TenantId)
			// assert.Equal() check permissions
			assert.Equal(user, req.RootUserId)
			assert.Equal(primaryGroup, req.RootGroupId)
			assert.Equal([]*quobyte.Label{
				{
					Name:  "k",
					Value: "v",
				},
				{
					Name:  "k1",
					Value: "v1",
				},
			}, req.Label)
			return &quobyte.CreateVolumeResponse{VolumeUuid: "created-volume-uuid"}, nil
		})
	mounter.EXPECT().Stat(gomock.Eq("created-volume-uuid/"+pvName)).Return(&ExistingDirMock{}, nil)
	req := &csi.CreateVolumeRequest{
		Name: pvName,
		Parameters: map[string]string{
			"quobyteTenant":     storageClassTenant,
			SharedVolumeNameKey: sharedVolumeName,
			"accessMode":        "750",
			"labels":            "k:v, k1:v1",
		},
		Secrets:       map[string]string{"a": "b"},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
	}
	resp, err := d.CreateVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal(
		storageClassTenant+"|created-volume-uuid|"+pvName,
		resp.Volume.VolumeId)
}

func TestCreateSnapshotVolume(t *testing.T) {
	assert := assert.New(t)
	req := &csi.CreateVolumeRequest{
		VolumeContentSource: nil,
	}
	// (nil, nil) indicate that the request is not snapshot volume request
	resp, err := createSnapshotResponse(req)
	assert.Nil(err)
	assert.Nil(resp)

	// not a k8 snapshot source
	req = &csi.CreateVolumeRequest{
		VolumeContentSource: &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{},
		},
	}
	resp, err = createSnapshotResponse(req)
	assert.Nil(err)
	assert.Nil(resp)

	snapshotId := "tenant|volume|snapshot"
	req = &csi.CreateVolumeRequest{
		Name:          "someName",
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
		VolumeContentSource: &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: snapshotId,
				},
			},
		},
	}
	resp, err = createSnapshotResponse(req)
	assert.Nil(err)
	assert.NotNil(resp)
	assert.Equal(SnapshotVolumeHandlePrefix+"someName", resp.GetVolume().VolumeId)
	assert.Equal(snapshotId, resp.Volume.VolumeContext[SnapshotIDKey])
	assert.Equal(snapshotId, resp.Volume.ContentSource.GetSnapshot().SnapshotId)
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
		VolumeId: tenant + VOLUME_HANDLE_PART_SEPARATOR + volume,
	}
	got, err = d.DeleteVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Nil(got)
	assert.True(strings.Contains(err.Error(), "secrets are required to delete a volume."))

	req = &csi.DeleteVolumeRequest{
		VolumeId: tenant + VOLUME_HANDLE_PART_SEPARATOR + volume,
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
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quoybteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	secrets := make(map[string]string)
	secrets[secretUserKey] = "some_management_user"
	secrets[secretPasswordKey] = "some_management_user_password"
	req = &csi.DeleteVolumeRequest{
		VolumeId: tenant + VOLUME_HANDLE_PART_SEPARATOR + volume,
		Secrets:  secrets,
	}
	got, err = d.DeleteVolume(context.TODO(), req)
	assert.Nil(err)
	assert.NotNil(got)
	assert.Equal(&csi.DeleteVolumeResponse{}, got)

	// Erase volume failure should send back error to upper layer
	ctrl = gomock.NewController(t)
	quoybteClient = mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quoybteClient.EXPECT().EraseVolumeByResolvingNamesToUUID(
		gomock.Eq(volume), gomock.Eq(tenant), gomock.Any()).Return(fmt.Errorf("Erase volume error"))
	quobyteClientProvider = mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quoybteClient, nil
		}).AnyTimes()
	d.quobyteClientFactory = quobyteClientProvider
	req = &csi.DeleteVolumeRequest{
		VolumeId: tenant + VOLUME_HANDLE_PART_SEPARATOR + volume,
		Secrets:  secrets,
	}
	got, err = d.DeleteVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Erase volume error")
	assert.Nil(got)
	// TODO(venkat): add snapshots and subdir (shared volume) tests
}

func TestDeleteSnapshotVolume(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	// Noop delete - snapshot does not have a backing volume for k8s snapshots
	req := &csi.DeleteVolumeRequest{
		VolumeId: SnapshotVolumeHandlePrefix + "test",
	}
	got, err := d.DeleteVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(got)
}

func TestDeleteVolumeSubdirUsingDeleteFilesTask(t *testing.T) {
	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	clientFactory := mocks.NewMockQuobyteApiClientProvider(ctrl)
	client := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	clientFactory.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).Return(
		client, nil).AnyTimes()
	d := &QuobyteDriver{
		UseDeleteFilesTask:   true,
		quobyteClientFactory: clientFactory,
	}
	client.EXPECT().CreateTask(gomock.Any()).DoAndReturn(
		func(taskReq *quobyte.CreateTaskRequest) (*quobyte.CreateTaskResponse, error) {
			assert.Equal(quobyte.TaskType_DELETE_FILES_IN_VOLUMES, taskReq.TaskType)
			assert.Equal("volume", taskReq.RestrictToVolumes[0])
			assert.Equal("/subdir", taskReq.DeleteFilesSettings.DirectoryPath)
			return &quobyte.CreateTaskResponse{}, nil
		})
	req := &csi.DeleteVolumeRequest{
		VolumeId: "tenant|volume|subdir",
		Secrets:  map[string]string{"a": "b"},
	}
	got, err := d.DeleteVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(got)

	client.EXPECT().CreateTask(gomock.Any()).DoAndReturn(
		func(taskReq *quobyte.CreateTaskRequest) (*quobyte.CreateTaskResponse, error) {
			assert.Equal(quobyte.TaskType_DELETE_FILES_IN_VOLUMES, taskReq.TaskType)
			assert.Equal("volume", taskReq.RestrictToVolumes[0])
			assert.Equal("/subdir", taskReq.DeleteFilesSettings.DirectoryPath)
			return nil, errors.New("some api error")
		})
	got, err = d.DeleteVolume(t.Context(), req)
	assert.Nil(got)
	assert.NotNil(err)
	assert.Contains(err.Error(), "some api error")
}

// uses rm on the client mount point
func TestDeleteVolumeSubdirUsingRemoveFiles(t *testing.T) {
	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	clientFactory := mocks.NewMockQuobyteApiClientProvider(ctrl)
	client := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	clientFactory.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).Return(
		client, nil).AnyTimes()
	mounter := mocks.NewMockMounter(ctrl)
	clientMountPath := "/someMountPath/"
	d := &QuobyteDriver{
		UseDeleteFilesTask:   false,
		quobyteClientFactory: clientFactory,
		mounter:              mounter,
		clientMountPoint:     clientMountPath,
		Name:                 "csi.quobyte.com",
	}

	mounter.EXPECT().Rename(gomock.Any(), gomock.Any()).DoAndReturn(
		func(old, new string) error {
			assert.Equal(old, clientMountPath+"volume/subdir")
			assert.Equal(new, clientMountPath+"volume/csi.quobyte.com_delete_subdir")
			return nil
		})
	req := &csi.DeleteVolumeRequest{
		VolumeId: "tenant|volume|subdir",
		Secrets:  map[string]string{"a": "b"},
	}
	response, err := d.DeleteVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(response)

	// Already deleted
	mounter.EXPECT().Rename(gomock.Any(), gomock.Any()).DoAndReturn(
		func(old, new string) error {
			assert.Equal(old, clientMountPath+"volume/subdir")
			assert.Equal(new, clientMountPath+"volume/csi.quobyte.com_delete_subdir")
			return &os.LinkError{
				Err: syscall.ENOENT,
			}
		})
	response, err = d.DeleteVolume(t.Context(), req)
	assert.Nil(err)
	assert.NotNil(response)

	mounter.EXPECT().Rename(gomock.Any(), gomock.Any()).DoAndReturn(
		func(old, new string) error {
			assert.Equal(old, clientMountPath+"volume/subdir")
			assert.Equal(new, clientMountPath+"volume/csi.quobyte.com_delete_subdir")
			return &os.LinkError{
				Err: syscall.EPERM,
			}
		})
	response, err = d.DeleteVolume(t.Context(), req)
	assert.Nil(response)
	assert.NotNil(err)
}

func TestControllerGetCapabilities(t *testing.T) {
	d := &QuobyteDriver{}
	resp, err := d.ControllerGetCapabilities(
		context.TODO(),
		&csi.ControllerGetCapabilitiesRequest{})
	assert := assert.New(t)
	assert.Nil(err)
	assert.NotNil(resp)
	gotCapabilities := parseCapabilities(resp.Capabilities)
	wanted := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	}
	assert.Equal(wanted, gotCapabilities)
}

func TestControllerModifyVolume(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	resp, err := d.ControllerModifyVolume(context.TODO(), &csi.ControllerModifyVolumeRequest{})
	assert.NotNil(err)
	assert.Contains(err.Error(), "Not implemented")
	assert.Nil(resp)
}

func TestControllerGetVolume(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	resp, err := d.ControllerGetVolume(context.TODO(), &csi.ControllerGetVolumeRequest{})
	assert.Nil(err)
	assert.NotNil(resp)
}

func TestControllerPublishVolume(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	resp, err := d.ControllerPublishVolume(context.TODO(), &csi.ControllerPublishVolumeRequest{})
	assert.Nil(err)
	assert.NotNil(resp)
}

func TestControllerUnpublishVolume(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	resp, err := d.ControllerUnpublishVolume(context.TODO(), &csi.ControllerUnpublishVolumeRequest{})
	assert.Nil(err)
	assert.NotNil(resp)
}

func TestDriverValidateVolumeCapabilities(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	resp, err := d.ValidateVolumeCapabilities(context.TODO(), &csi.ValidateVolumeCapabilitiesRequest{})
	assert.NotNil(err)
	assert.Contains(err.Error(), "Not implemented")
	assert.Nil(resp)
}

func TestListVolumes(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	resp, err := d.ListVolumes(context.TODO(), &csi.ListVolumesRequest{})
	assert.NotNil(err)
	assert.Contains(err.Error(), "Not implemented")
	assert.Nil(resp)
}

func TestGetCapacity(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	resp, err := d.GetCapacity(context.TODO(), &csi.GetCapacityRequest{})
	assert.NotNil(err)
	assert.Contains(err.Error(), "Not implemented")
	assert.Nil(resp)
}

func TestControllerExpandVolume(t *testing.T) {
	assert := assert.New(t)
	d := &QuobyteDriver{}
	req := &csi.ControllerExpandVolumeRequest{}
	resp, err := d.ControllerExpandVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Contains(err.Error(), "invalid capacity range nil for expand volume")
	assert.Nil(resp)

	req.CapacityRange = &csi.CapacityRange{}
	req.CapacityRange.RequiredBytes = int64(100)
	resp, err = d.ControllerExpandVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Contains(err.Error(), "is not in the form")
	assert.Nil(resp)

	req.VolumeId = "tenant|volume"
	resp, err = d.ControllerExpandVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Contains(err.Error(), "controller-expand-secret-name and controller-expand-secret")
	assert.Nil(resp)

	req.Secrets = map[string]string{
		secretUserKey:     "user",
		secretPasswordKey: "pass",
	}
	ctrl := gomock.NewController(t)
	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(
		gomock.Any(), gomock.Any()).Return("some-volume-uuid", nil)
	quobyteClient.EXPECT().SetVolumeQuota(
		gomock.Any(), gomock.Any()).Return(nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return nil, fmt.Errorf("cannot create a client")
		})
	d.quobyteClientFactory = quobyteClientProvider
	resp, err = d.ControllerExpandVolume(context.TODO(), req)
	assert.NotNil(err)
	assert.Contains(err.Error(), "cannot create a client")
	assert.Nil(resp)

	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		})

	resp, err = d.ControllerExpandVolume(context.TODO(), req)
	assert.Nil(err)
	assert.NotNil(resp)

	req.VolumeId = "tenant|volume|snapshot-dir"
	resp, err = d.ControllerExpandVolume(context.TODO(), req)
	assert.Nil(err)
	assert.NotNil(resp)
}

func TestDeleteSnapshot(t *testing.T) {
	assert := assert.New(t)
	req := &csi.DeleteSnapshotRequest{}
	req.SnapshotId = "tenant|volume|snapshot"
	ctrl := gomock.NewController(t)
	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(
		gomock.Any(), gomock.Any()).Return("some-volume-uuid", nil)
	quobyteClient.EXPECT().GetTenantUUID(gomock.Any()).Return("some-tenant-uuid", nil)
	quobyteClient.EXPECT().DeleteSnapshot(gomock.Any()).Return(
		&quobyte.DeleteSnapshotResponse{}, nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		})
	d := &QuobyteDriver{}
	d.quobyteClientFactory = quobyteClientProvider
	resp, err := d.DeleteSnapshot(context.TODO(), req)
	assert.NotNil(resp)
	assert.Nil(err)
}

func TestCreateSnapshots(t *testing.T) {
	ctrl := gomock.NewController(t)
	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(
		gomock.Any(), gomock.Any()).Return("volume", nil)
	quobyteClient.EXPECT().GetTenantUUID(gomock.Any()).Return("tenant", nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClient.EXPECT().CreateSnapshot(gomock.Any()).Return(
		&quobyte.CreateSnapshotResponse{}, nil)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		})
	d := &QuobyteDriver{}
	d.quobyteClientFactory = quobyteClientProvider
	snapshotName := "snapshot1"
	req := &csi.CreateSnapshotRequest{
		Name:           snapshotName,
		SourceVolumeId: "tenant|volume",
		Parameters:     map[string]string{},
	}
	response, _ := d.CreateSnapshot(t.Context(), req)
	snapshot := response.Snapshot
	assert := assert.New(t)
	assert.NotNil(snapshot)
	assert.Equal("tenant|volume|snapshot1", snapshot.SnapshotId)
}

func TestListSnapshots(t *testing.T) {
	ctrl := gomock.NewController(t)
	quobyteClient := mock_quobyte_api.NewMockExtendedQuobyteApi(ctrl)
	quobyteClient.EXPECT().GetVolumeUUID(
		gomock.Any(), gomock.Any()).Return("some-volume-uuid", nil)
	quobyteClient.EXPECT().GetTenantUUID(gomock.Any()).Return("some-tenant-uuid", nil)
	quobyteClientProvider := mocks.NewMockQuobyteApiClientProvider(ctrl)
	quobyteClient.EXPECT().ListSnapshots(gomock.Any()).Return(
		&quobyte.ListSnapshotsResponse{
			Snapshot: []*quobyte.VolumeSnapshot{
				{Name: "snapshot1"},
				{Name: "snapshot2"},
			},
		}, nil)
	quobyteClientProvider.EXPECT().NewQuobyteApiClient(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *url.URL, _ map[string]string) (quobyte.ExtendedQuobyteApi, error) {
			return quobyteClient, nil
		})
	d := &QuobyteDriver{}
	d.quobyteClientFactory = quobyteClientProvider
	req := &csi.ListSnapshotsRequest{
		SnapshotId: "tenant|volume|snapshot",
	}
	response, _ := d.ListSnapshots(context.TODO(), req)
	snapshots := response.Entries
	assert := assert.New(t)
	assert.NotNil(snapshots)
	assert.Equal("tenant|volume|snapshot1", snapshots[0].Snapshot.SnapshotId)
	assert.Equal("tenant|volume|snapshot2", snapshots[1].Snapshot.SnapshotId)
}

func parseCapabilities(capabilities []*csi.ControllerServiceCapability) []csi.ControllerServiceCapability_RPC_Type {
	var caps []csi.ControllerServiceCapability_RPC_Type
	for _, capability := range capabilities {
		caps = append(caps, capability.GetRpc().Type)
	}
	return caps
}
