package driver

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"k8s.io/klog"
)

//go:generate mockgen -package=mocks -destination  ../mocks/mock_mounter.go github.com/quobyte/quobyte-csi-driver/driver Mounter
type Mounter interface {
	CreateMountPath(path string) error
	Mount(source, target string) error
	Unmount(path string) error
	Statfs(path string) (unix.Statfs_t, error)
}

type LinuxMounter struct {
}

func (m *LinuxMounter) CreateMountPath(mountPath string) error {
	if err := os.MkdirAll(mountPath, 0750); err != nil {
		return err
	}
	return nil
}

func (m *LinuxMounter) Mount(source, target string) error {
	klog.Infof("Executing bind mount for source:'%s' target: '%s'", source, target)
	if err := unix.Mount(source, target, "" /*bind mount*/, unix.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed bind mount of source: '%s' target: '%s' due to: %s", source, target, err)
	}
	return nil
}

func (m *LinuxMounter) Unmount(path string) error {
	if len(path) == 0 {
		return errors.New("Given unmount path is empty.")
	}
	if err := unix.Unmount(path, 0 /*normal unmount - not a lazy unmount*/); err != nil {
		klog.Errorf("failed unmount of %s due to %s", path, err)
	}
	return nil
}

func (m *LinuxMounter) Statfs(path string) (unix.Statfs_t, error) {
	var statfs unix.Statfs_t
	if err := unix.Statfs(path, &statfs); err != nil {
		return unix.Statfs_t{}, err
	}
	return statfs, nil
}

// Mount bind mounts the Quobyte volume to the target
func Mount(source, target string, mounter Mounter) error {
	// Readonly is left to kubelet running on host machines.
	// Remounting from the pods is not allowed from the running container
	// https://github.com/moby/moby/issues/31591
	// For fuse FS, remount does not fail but making whole subtree readonly
	// on remount with -o remount,ro,bind and we don't want that.

	// We bind mount the host Quobyte path into the pod
	if err := mounter.Mount(source, target); err != nil {
		return err
	}
	return nil
}

// Unmount unmounts the given path
func Unmount(target string, mounter Mounter) error {
	return mounter.Unmount(target)
}
