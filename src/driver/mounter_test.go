package driver

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/quobyte/quobyte-csi-driver/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestMount(t *testing.T) {
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	wantedSource := "/some/source"
	wantedTarget := "/some/target"
	mounter.EXPECT().Mount(gomock.Any(), gomock.Any()).DoAndReturn(
		func(gotSource string, gotTarget string) error {
			if wantedSource != gotSource {
				t.Errorf("wanted: %v but got: %v", wantedSource, gotSource)
			}
			if wantedTarget != gotTarget {
				t.Errorf("wanted: %v but got: %v", wantedTarget, gotTarget)
			}
			return nil
		})
	Mount(wantedSource, wantedTarget, mounter)
}

func TestUnmount(t *testing.T) {
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	assert := assert.New(t)

	mounter.EXPECT().Unmount(gomock.Any()).Return(fmt.Errorf("Some error"))
	err := Unmount("/some/pod/mount/path", mounter)
	assert.NotNil(err)

	mounter.EXPECT().Unmount(gomock.Any()).Return(fmt.Errorf("Some error"))
	err = Unmount("", mounter)
	assert.NotNil(err)

	mounter.EXPECT().Unmount(gomock.Any()).DoAndReturn(
		func(got string) error {
			wanted := "/some/pod/mount/path"
			if wanted != got {
				t.Errorf("wanted: %s but got %s", wanted, got)
			}
			return nil
		})
	Unmount("/some/pod/mount/path", mounter)
}

func TestCreateAccessKeyContextHandle(t *testing.T) {
	assert := assert.New(t)
	mounter := &LinuxMounter{}
	accessKeyHandle := mounter.CreateAccessKeyContextHandle()
	_, err := uuid.Parse(accessKeyHandle)
	assert.Nil(err)
}
