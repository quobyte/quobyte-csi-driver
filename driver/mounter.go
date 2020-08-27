package driver

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"k8s.io/klog"
)

// Mount bind mounts the Quobyte volume to the target
func Mount(source, target, fsType string, opts []string) error {
	// Readonly is left to kubelet running on host machines.
	// Remounting from the pods is not allowed from the running container
	// https://github.com/moby/moby/issues/31591
	// For fuse FS, remount does not fail but making whole subtree readonly
	// on remount with -o remount,ro,bind and we don't want that.

	cmd := "mount"
	var options []string
	options = append(options, "-o")
	opts = append(opts, "bind")
	options = append(options, strings.Join(opts, ","))
	options = append(options, source)
	options = append(options, target)
	klog.Infof("Executing mount command '%s %s'", cmd, strings.Join(options, " "))
	if out, err := exec.Command(cmd, options...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed mount: %v cmd: '%s %s %s' command output: %q", err, cmd, options, target, string(out))
	}
	return nil
}

//Unmount unmounts the given path
func Unmount(target string) error {
	cmd := "umount"
	if len(target) == 0 {
		return errors.New("Given unmount path is empty")
	}
	if out, err := exec.Command(cmd, target).CombinedOutput(); err != nil {
		klog.Errorf("failed unmount: %v cmd: '%s %s' command output: %q", err, cmd, target, string(out))
	}
	return nil
}
