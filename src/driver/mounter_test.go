package driver

import (
	"testing"

	"github.com/quobyte/quobyte-csi-driver/mocks"
	"go.uber.org/mock/gomock"
)

func TestMount(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mocks.NewMockMounter(ctrl)
	wantedSource := "/some/source"
	wantedTarget := "/some/target"
	m.EXPECT().Mount(gomock.Any(), gomock.Any()).DoAndReturn(
		func(gotSource string, gotTarget string) error {
			if wantedSource != gotSource {
				t.Errorf("wanted: %v but got: %v", wantedSource, gotSource)
			}
			if wantedTarget != gotTarget {
				t.Errorf("wanted: %v but got: %v", wantedTarget, gotTarget)
			}
			return nil
		})
	Mount(wantedSource, wantedTarget, m)
}

func TestUnmount(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mocks.NewMockMounter(ctrl)
	m.EXPECT().Unmount(gomock.Any()).DoAndReturn(
		func(got string) error {
			wanted := "/some/pod/mount/path"
			if wanted != got {
				t.Errorf("wanted: %s but got %s", wanted, got)
			}
			return nil
		})
	Unmount("/some/pod/mount/path", m)
}
