package driver

import (
	"reflect"
	"testing"

	"github.com/quobyte/quobyte-csi-driver/mocks"
	"go.uber.org/mock/gomock"
)

func TestMount(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mocks.NewMockMounter(ctrl)
	m.EXPECT().Mount(gomock.Any()).DoAndReturn(
		func(opts []string) error {
			wanted := []string{"-o", "opt1,opt2,bind", "some-source", "some-target"}
			if !reflect.DeepEqual(wanted, opts) {
				t.Errorf("wanted: %v but got: %v", wanted, opts)
			}
			return nil
		});
	Mount("some-source", "some-target", []string{"opt1", "opt2"}, m)
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
		});
	Unmount("/some/pod/mount/path", m)
}