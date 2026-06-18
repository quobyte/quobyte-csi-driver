package driver

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"syscall"
	"testing"

	"github.com/quobyte/api/v4/quobyte"
	"github.com/quobyte/quobyte-csi-driver/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestChownDirectory_UserResolutionFailure(t *testing.T) {
	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	mounter.EXPECT().GetNodeName().Return("dummy-node")
	req := &quobyte.CreateVolumeRequest{}

	mounter.EXPECT().Lookup(gomock.Any()).Return(&user.User{Uid: "invalid"}, nil)
	err := chownDirectory("someDirPath", req, mounter)
	assert.NotNil(err)

	// user resolution error
	mounter.EXPECT().Lookup(gomock.Any()).Return(
		&user.User{},
		errors.New("user resolution failure"))
	err = chownDirectory("someDirPath", req, mounter)
	assert.NotNil(err)
	assert.Contains(err.Error(), "user resolution failure")
}

func TestChownDirectory_GroupResolutionFailure(t *testing.T) {
	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	mounter.EXPECT().GetNodeName().Return("dummy-node")
	req := &quobyte.CreateVolumeRequest{}

	mounter.EXPECT().Lookup(gomock.Any()).Return(&user.User{Uid: "0"}, nil).AnyTimes()
	mounter.EXPECT().LookupGroup(gomock.Any()).Return(&user.Group{Gid: "invalid"}, nil)
	err := chownDirectory("someDirPath", req, mounter)
	assert.NotNil(err)

	// group resolution error
	mounter.EXPECT().LookupGroup(gomock.Any()).Return(&user.Group{},
		errors.New("group resolution failure"))
	err = chownDirectory("someDirPath", req, mounter)
	assert.NotNil(err)
	assert.Contains(err.Error(), "group resolution failure")
}

func TestChownDirectory(t *testing.T) {
	assert := assert.New(t)
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	mounter.EXPECT().GetNodeName().Return("dummy-node")
	mounter.EXPECT().Lookup(gomock.Any()).Return(&user.User{Uid: "0"}, nil).AnyTimes()
	mounter.EXPECT().LookupGroup(gomock.Any()).Return(&user.Group{Gid: "0"}, nil).AnyTimes()
	mounter.EXPECT().Chown(gomock.Eq("sanePath"), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	req := &quobyte.CreateVolumeRequest{}
	err := chownDirectory("sanePath", req, mounter)
	assert.Nil(err)

	mounter.EXPECT().Chown(gomock.Eq("errorPath"), gomock.Any(), gomock.Any()).Return(
		errors.New("failed chown")).Times(1)
	err = chownDirectory("errorPath", req, mounter)
	assert.NotNil(err)
	fmt.Printf("%s", err)
	assert.Contains(err.Error(), "failed chown")
}

func TestCreateAndOwnDirectory(t *testing.T) {
	assert := assert.New(t)

	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	req := &quobyte.CreateVolumeRequest{
		AccessMode: 800,
	}
	err := createDirectory("sanePath", req, mounter)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Cannot parse access mode")

	gomock.InOrder(
		mounter.EXPECT().Mkdir(gomock.Any(), gomock.Any()).Return(nil),
		mounter.EXPECT().Mkdir(gomock.Any(), gomock.Any()).Return(&os.PathError{
			Err: syscall.EEXIST,
		}))
	mounter.EXPECT().Chmod(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	mounter.EXPECT().Lookup(gomock.Any()).Return(&user.User{Uid: "0"}, nil).Times(2)
	mounter.EXPECT().LookupGroup(gomock.Any()).Return(&user.Group{Gid: "0"}, nil).Times(2)
	mounter.EXPECT().Chown(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
	req = &quobyte.CreateVolumeRequest{
		AccessMode:  700,
		RootUserId:  "someUser",
		RootGroupId: "someGroup",
	}
	err = createDirectory("sanePath", req, mounter)
	assert.Nil(err)

	err = createDirectory("existingPath", req, mounter)
	assert.Nil(err)
}

func TestCreateAndOwnDirectory_Error(t *testing.T) {
	assert := assert.New(t)

	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)

	gomock.InOrder(
		mounter.EXPECT().Mkdir(gomock.Any(), gomock.Any()).Return(&os.PathError{
			Err: syscall.ENODEV,
		}),
		mounter.EXPECT().Mkdir(gomock.Any(), gomock.Any()).Return(nil),
	)
	req := &quobyte.CreateVolumeRequest{
		AccessMode:  700,
		RootUserId:  "someUser",
		RootGroupId: "someGroup",
	}
	err := createDirectory("existingPath", req, mounter)
	assert.NotNil(err)

	mounter.EXPECT().Chmod(gomock.Any(), gomock.Any()).Return(errors.New("some chomod error"))
	err = createDirectory("deviceErrorOnMkDir", req, mounter)
	assert.NotNil(err)
	assert.Contains(err.Error(), "Cannot apply requested permissions")
}

type ExistingFileMock struct {
	os.FileInfo
	name string
}

func (f *ExistingFileMock) IsDir() bool {
	return false
}

func TestMkDir_Error(t *testing.T) {
	assert := assert.New(t)

	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)

	gomock.InOrder(
		mounter.EXPECT().Stat(gomock.Any()).Return(&ExistingFileMock{}, nil),
		mounter.EXPECT().Stat(gomock.Any()).Return(nil, &os.PathError{
			Err: syscall.ENOTCONN,
		}),
	)
	req := &quobyte.CreateVolumeRequest{
		AccessMode:  700,
		RootUserId:  "someUser",
		RootGroupId: "someGroup",
	}
	maker := &DirectoryMaker{mounter: mounter}
	err := maker.Mkdir("existingPathIsAFile", req)
	assert.NotNil(err)
	assert.Contains(err.Error(), "A file with sub-directory name exists at")

	err = maker.Mkdir("disconnectedMountPointError", req)
	assert.NotNil(err)
}

func TestMkDirDirectory(t *testing.T) {
	assert := assert.New(t)

	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	mounter.EXPECT().Stat(gomock.Any()).Return(nil, &os.PathError{
		Err: syscall.ENOENT,
	}).AnyTimes()
	gomock.InOrder(
		mounter.EXPECT().Mkdir(gomock.Any(), gomock.Any()).Return(nil),
		mounter.EXPECT().Mkdir(gomock.Any(), gomock.Any()).Return(&os.PathError{
			Err: syscall.ENOTCONN,
		}))
	mounter.EXPECT().Chmod(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mounter.EXPECT().Lookup(gomock.Any()).Return(&user.User{Uid: "0"}, nil).Times(1)
	mounter.EXPECT().LookupGroup(gomock.Any()).Return(&user.Group{Gid: "0"}, nil).Times(1)
	mounter.EXPECT().Chown(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	req := &quobyte.CreateVolumeRequest{
		AccessMode:  700,
		RootUserId:  "someUser",
		RootGroupId: "someGroup",
	}
	maker := &DirectoryMaker{mounter: mounter}
	err := maker.Mkdir("createADir", req)
	assert.Nil(err)

	err = maker.Mkdir("failCreation", req)
	assert.NotNil(err)
}
