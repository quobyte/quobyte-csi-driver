package driver

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/golang/glog"
)

// Mount bind mounts the Quobyte volume to the target
func Mount(source, target, fsType string, opts []string) error {
	cmd := "mount"
	var options []string
	options = append(options, "-o")
	opts = append(opts, "bind")
	options = append(options, strings.Join(opts, ","))
	options = append(options, source)
	options = append(options, target)
	glog.Infof("Executing mount command '%s %s'", cmd, strings.Join(options, " "))
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
		return fmt.Errorf("failed unmount: %v cmd: '%s %s' command output: %q", err, cmd, target, string(out))
	}
	return nil
}
