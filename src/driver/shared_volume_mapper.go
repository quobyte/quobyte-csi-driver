package driver

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/quobyte/api/v4/quobyte"
)

type DirectoryCreator interface {
	Mkdir(path string, req *quobyte.CreateVolumeRequest) error
}

type DirectoryMaker struct {
	mounter Mounter
}

func (maker *DirectoryMaker) Mkdir(path string, req *quobyte.CreateVolumeRequest) error {
	//
	statInfo, err := maker.mounter.Stat(path)
	if err != nil {
		if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOENT {
			if err = createDirectory(path, req, maker.mounter); err != nil {
				return err
			}
			return nil
		} else {
			return err
		}
	}
	if !statInfo.IsDir() {
		return fmt.Errorf("A file with sub-directory name exists at %s", path)
	}
	return nil
}

func createDirectory(
	path string, volRequest *quobyte.CreateVolumeRequest, mounter Mounter) error {
	modeVal, err := strconv.ParseUint(strconv.Itoa(int(volRequest.AccessMode)), 8, 32)
	if err != nil {
		return fmt.Errorf("Cannot parse access mode due to %s", err)
	}
	if err = mounter.Mkdir(path, fs.FileMode(modeVal)); err != nil {
		// ignore directory exists error; might have been created by replicated pods
		if e, ok := err.(*os.PathError); ok && e.Err != syscall.EEXIST {
			return fmt.Errorf("Unable to create sub-directory %s due to %s", path, err)
		}
	}
	if err = mounter.Chmod(path, fs.FileMode(modeVal)); err != nil {
		return fmt.Errorf("Cannot apply requested permissions %d for %s due to %s",
			modeVal, path, err)
	}
	return chownDirectory(path, volRequest, mounter)
}

func chownDirectory(
	path string, req *quobyte.CreateVolumeRequest, mounter Mounter) error {
	var userInfo *user.User
	var groupInfo *user.Group
	var err error
	if userInfo, err = mounter.Lookup(req.RootUserId); err != nil {
		return fmt.Errorf("Cannot look up user '%s' on node '%s' due to error %s",
			req.RootUserId, mounter.GetNodeName(), err)
	}
	var userId, groupId int
	if userId, err = strconv.Atoi(userInfo.Uid); err != nil {
		return err
	}
	if groupInfo, err = mounter.LookupGroup(req.RootGroupId); err != nil {
		return fmt.Errorf("Cannot look up group '%s' on node '%s' due to error %s",
			req.RootGroupId, mounter.GetNodeName(), err)
	}
	if groupId, err = strconv.Atoi(groupInfo.Gid); err != nil {
		return err
	}
	if err := mounter.Chown(path, userId, groupId); err != nil {
		return fmt.Errorf("Cannot change ownership of '%s' to '%s:%s' on node '%s' due to %s",
			path, req.RootUserId, req.RootGroupId, mounter.GetNodeName(), err)
	}
	return nil
}
