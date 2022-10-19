package driver

import (
	"context"
	"fmt"
	"os"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/sys/unix"
	"k8s.io/klog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	xattrKey     string = "quobyte.access_key"
	empty_string string = ""
	snapshotsDir string = ".snapshots"
)

// NodePublishVolume mounts the volume to the pod with the given target path
// QuobyteClient does the mounting of the volumes
func (d *QuobyteDriver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	var volumeId string

	var snapshotName string = empty_string
	volContext := req.GetVolumeContext()
	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, fmt.Errorf("given target mount path is empty")
	}

	if err := os.MkdirAll(targetPath, 0750); err != nil {
		return nil, err
	}

	// see controller.go -- CreateVolume method. VolumeContext is only added for snapshot volumes
	if volContext != nil && strings.HasPrefix(req.VolumeId, SnapshotVolumeHandlePrefix) {
		if snapshotId, ok := volContext[SnapshotIDKey]; ok {
			snapshotParts := strings.Split(snapshotId, SEPARATOR)
			if len(snapshotParts) < 3 {
				return nil, getInvalidSnapshotIdError(snapshotId)
			}
			if len(snapshotParts) == 4 {
				volumeId = snapshotParts[0] + SEPARATOR + snapshotParts[1] + SEPARATOR + snapshotParts[3]
			} else {
				volumeId = snapshotParts[0] + SEPARATOR + snapshotParts[1]
			}
			snapshotName = snapshotParts[2]
		}
	} else {
		volumeId = req.GetVolumeId()
	}
	// Incase of preprovisioned volumes, NodePublishSecrets are not taken from storage class but
	// needs to be passed as nodePublishSecretRef in PV (kubernetes) definition
	secrets := req.GetSecrets()
	volParts := strings.Split(volumeId, SEPARATOR)
	if len(volParts) < 2 {
		return nil, fmt.Errorf("given volumeHandle '%s' is not in the format <TENANT_NAME/TENANT_UUID>%s<VOL_NAME/VOL_UUID>", volumeId, SEPARATOR)
	}

	var volUUID string
	if len(secrets) == 0 || !hasApiCredentials(secrets) {
		// cannot resolve volume Id without Quobyte API credentials if tenant name & volume name is given..assume volume uuid
		klog.Infof("csiNodePublishSecret is  not received with sufficient Quobyte API credential. Assuming volume given with UUID")
		volUUID = volParts[1]
	} else {
		quobyteClient, err := d.getQuobyteApiClient(secrets)
		if err != nil {
			return nil, err
		}
		// volume name should be retrieved from the req.GetVolumeId()
		// Due to csi lacking in parameter passing during delete Volume, req.volumeId is changed
		// to <TENANT_NAME/TENANT_UUID>|<VOL_NAME/VOL_UUID>. see controller.go CreateVolume for the details.
		volUUID, err = quobyteClient.GetVolumeUUID(volParts[1], volParts[0])
		if err != nil {
			return nil, err
		}
	}

	var options []string
	volCap := req.GetVolumeCapability()
	if volCap != nil {
		mount := volCap.GetMount()
		if mount != nil {
			mntFlags := mount.GetMountFlags()
			if mntFlags != nil {
				options = mntFlags
			}
		}
	}
	var mountPath string
	if d.QuobyteVersion >= 3 && d.IsQuobyteAccessKeyMountsEnabled {
		podUUID := getSanitizedPodUUIDFromPath(targetPath)
		accesskeyID, ok := secrets[accessKeyID]
		if !ok {
			return nil, fmt.Errorf("Mount secret should have '%s: <YOUR_ACCESS_KEY_ID>'", accessKeyID)
		}
		accesskeySecret, ok := secrets[accessKeySecret]
		if !ok {
			return nil, fmt.Errorf("Mount secret should have '%s: <YOUR_ACCESS_KEY_SECRET>'", accessKeySecret)
		}
		accesskeyHandle := fmt.Sprintf("%s-%s", podUUID, accesskeyID)
		XattrVal := getAccessKeyValStr(accesskeyID, accesskeySecret, accesskeyHandle)
		err := setfattr(xattrKey, XattrVal, fmt.Sprintf("%s/%s", d.clientMountPoint, volUUID))
		if err != nil {
			return nil, err
		}
		if snapshotName == empty_string {
			if len(volParts) == 3 { // tenant|volume|subDir
				mountPath = fmt.Sprintf("%s/%s@%s/%s", d.clientMountPoint, accesskeyHandle, volUUID, volParts[2])
			} else {
				mountPath = fmt.Sprintf("%s/%s@%s", d.clientMountPoint, accesskeyHandle, volUUID)
			}
		} else {
			// We  tenant|volume|snapshot|subDir
			if len(volParts) == 3 { // tenant|volume|subDir
				mountPath = fmt.Sprintf("%s/%s@%s/%s/%s/%s", d.clientMountPoint, accesskeyHandle, volUUID, snapshotsDir, snapshotName, volParts[2])
			} else {
				mountPath = fmt.Sprintf("%s/%s@%s/%s/%s", d.clientMountPoint, accesskeyHandle, volUUID, snapshotsDir, snapshotName)
			}
		}
	} else {
		if snapshotName == empty_string {
			if len(volParts) == 3 { // tenant|volume|subDir
				mountPath = fmt.Sprintf("%s/%s/%s", d.clientMountPoint, volUUID, volParts[2])
			} else { // tenant|volume
				mountPath = fmt.Sprintf("%s/%s", d.clientMountPoint, volUUID)
			}
		} else {
			if len(volParts) == 3 { // tenant|volume|subDir
				mountPath = fmt.Sprintf("%s/%s/%s/%s/%s", d.clientMountPoint, volUUID, snapshotsDir, snapshotName, volParts[2])
			} else { // tenant|volume
				mountPath = fmt.Sprintf("%s/%s/%s/%s", d.clientMountPoint, volUUID, snapshotsDir, snapshotName)
			}
		}
	}
	err := Mount(mountPath, targetPath, options)
	if err != nil {
		return nil, err
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume Currently not implemented as Quobyte has only single mount point
func (d *QuobyteDriver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, fmt.Errorf("target path for unmount is empty")
	}
	klog.Infof("Unmounting %s", target)
	err := Unmount(target)
	if err != nil {
		return nil, err
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities returns the capabilities of the node server
func (d *QuobyteDriver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
		},
	}, nil
}

// NodeStageVolume Stages the volume to the node under /mnt/quobyte
func (d *QuobyteDriver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "NodeStageVolume: Not implented by Quobyte CSI")
}

// NodeUnstageVolume Unstages the volume from /mnt/quobyte
func (d *QuobyteDriver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "NodeUnstageVolume: Not implented by Quobyte CSI")
}

func (d *QuobyteDriver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: d.NodeName,
	}, nil
}

func (d *QuobyteDriver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "NodeExpandVolume: Not implented by Quobyte CSI")
}

func (d *QuobyteDriver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	volumePath := req.GetVolumePath()
	if len(volumePath) <= 0 {
		return nil, fmt.Errorf("volume path must not be empty")
	}
	var statfs unix.Statfs_t
	if err := unix.Statfs(volumePath, &statfs); err != nil {
		return nil, err
	}

	usedBytes := (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize)
	availableBytes := int64(statfs.Bavail) * int64(statfs.Bsize)
	totalBytes := int64(statfs.Blocks) * int64(statfs.Bsize)

	usedInodes := int64(statfs.Files) - int64(statfs.Ffree)
	availableInodes := int64(statfs.Ffree)
	totalInodes := int64(statfs.Files)

	resp := &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Used:      usedBytes,
				Available: availableBytes,
				Total:     totalBytes,
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Used:      usedInodes,
				Available: availableInodes,
				Total:     totalInodes,
			},
		},
	}
	return resp, nil
}
