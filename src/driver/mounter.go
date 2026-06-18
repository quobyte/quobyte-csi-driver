package driver

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"syscall"

	"github.com/google/uuid"
	"github.com/pkg/xattr"
	"golang.org/x/sys/unix"

	"k8s.io/klog"
)

//go:generate mockgen -package=mocks -destination  ../mocks/mock_mounter.go github.com/quobyte/quobyte-csi-driver/driver Mounter
type Mounter interface {
	Mkdirs(path string, permissions os.FileMode) error
	Mkdir(path string, permissions os.FileMode) error
	Chmod(path string, permissions os.FileMode) error
	Chown(path string, userId, groupId int) error
	Rename(oldPath, newPath string) error
	Lookup(userName string) (*user.User, error)
	LookupGroup(groupName string) (*user.Group, error)
	Mount(source, target string) error
	Unmount(path string) error
	Stat(path string) (os.FileInfo, error)
	Statfs(path string) (unix.Statfs_t, error)
	SetXAttr(key, val, mountPath string) error
	CreateAccessKeyContextHandle() string
	GetNodeName() string
	RemoveAll(path string) error
}

type LinuxMounter struct {
	NodeName string
}

func (m *LinuxMounter) GetNodeName() string {
	return m.NodeName
}

func (m *LinuxMounter) CreateAccessKeyContextHandle() string {
	return uuid.New().String()
}

func (m *LinuxMounter) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (m *LinuxMounter) Chown(path string, userId, groupId int) error {
	return os.Chown(path, userId, groupId)
}

func (m *LinuxMounter) Chmod(path string, permissions os.FileMode) error {
	return os.Chmod(path, permissions)
}

func (m *LinuxMounter) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (m *LinuxMounter) Lookup(userName string) (*user.User, error) {
	return user.Lookup(userName)
}

func (m *LinuxMounter) LookupGroup(groupName string) (*user.Group, error) {
	return user.LookupGroup(groupName)
}

func (m *LinuxMounter) SetXAttr(key, val, path string) error {
	return xattr.Set(path, key, []byte(val))
}

func (m *LinuxMounter) Mkdirs(path string, permissions os.FileMode) error {
	return os.MkdirAll(path, permissions)
}

func (m *LinuxMounter) Mkdir(path string, permissions os.FileMode) error {
	return os.Mkdir(path, permissions)
}

func (m *LinuxMounter) Mount(source, target string) error {
	klog.V(5).Infof("Executing bind mount for source: '%s' target: '%s'", source, target)
	if err := unix.Mount(source, target, "" /*bind mount*/, unix.MS_BIND, ""); err != nil {
		return fmt.Errorf("failed bind mount of source: '%s' target: '%s' due to: %s", source, target, err)
	}
	return nil
}

func (m *LinuxMounter) Unmount(path string) error {
	if len(path) == 0 {
		return errors.New("Given unmount path is empty.")
	}
	err := unix.Unmount(path, 0 /*normal unmount - not a lazy unmount*/)
	if err == nil {
		return nil
	}
	errno := err.(syscall.Errno)
	// Bind bound results in ENOTCONN if client restarts or killed.
	// Such mounts cannot be unmounted gracefully. Pod needs to be deleted (with pod killer)
	// Do not return this error as k8s keep retrying unmount - which never succeeds.
	if errno == unix.ENOTCONN {
		klog.Infof("mount point %s is no longer connected. So, no need to unmount it.", path)
		return nil
	}
	klog.Errorf("failed unmount of %s due to %s", path, err)
	return err
}

func (m *LinuxMounter) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
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
