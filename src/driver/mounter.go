package driver

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"k8s.io/klog"
)

//go:generate mockgen -package=mocks -destination  ../mocks/mock_mounter.go github.com/quobyte/quobyte-csi-driver/driver Mounter
type Mounter interface {
	Mount(mount_options []string) error
	Unmount(path string) error
}

type LinuxMounter struct {
}

func (m *LinuxMounter) Mount(mount_options []string) error {
	cmd := "mount"
	klog.Infof("Executing mount command '%s %s'", cmd, strings.Join(mount_options, " "))
	if out, err := exec.Command(cmd, mount_options...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed mount: %v cmd: '%s %s' command output: %q", err, cmd, mount_options, string(out))
	}
	return nil
}

func (m *LinuxMounter) Unmount(path string) error {
	if len(path) == 0 {
		return errors.New("Given unmount path is empty.")
	}
	cmd := "umount"
	if out, err := exec.Command(cmd, path).CombinedOutput(); err != nil {
		klog.Errorf("failed unmount: %v cmd: '%s %s' command output: %q", err, cmd, path, string(out))
	}
	return nil
}

// Mount bind mounts the Quobyte volume to the target
func Mount(source, target string, opts []string, mounter Mounter) error {
	// Readonly is left to kubelet running on host machines.
	// Remounting from the pods is not allowed from the running container
	// https://github.com/moby/moby/issues/31591
	// For fuse FS, remount does not fail but making whole subtree readonly
	// on remount with -o remount,ro,bind and we don't want that.

	// We bind mount the host Quobyte path into the pod
	opts = append(opts, "bind")
	var options []string
	options = append(options, "-o")
	options = append(options, strings.Join(opts, ","))
	options = append(options, source)
	options = append(options, target)
	if err := mounter.Mount(options); err != nil {
		return err
	}
	return nil
}

//Unmount unmounts the given path
func Unmount(target string, mounter Mounter) error {
	return mounter.Unmount(target)
}
