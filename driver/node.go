package driver

import (
	"context"
	"fmt"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	xattrKey        string = "quobyte.access_key"
	accessKeyID     string = "accessKeyId"
	accessKeySecret string = "accessKeySecret"
	empty_string    string = ""
	snapshotsDir    string = ".snapshots"
)

// NodePublishVolume mounts the volume to the pod with the given target path
// QuobyteClient does the mounting of the volumes
func (d *QuobyteDriver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	var volumeId string

	var snapshotName string = empty_string
	volContext := req.GetVolumeContext()
	targetPath := req.GetTargetPath()
	readonly := req.Readonly
	// see controller.go -- CreateVolume method. VolumeContext is only added for snapshot volumes
	if volContext != nil && strings.HasPrefix(req.VolumeId, SnapshotVolumeHandlePrefix) {
		if snapshotId, ok := volContext[SnapshotIDKey]; ok {
			snapshotParts := strings.Split(snapshotId, SEPARATOR)
			if len(snapshotParts) < 3 {
				return nil, getInvlaidSnapshotIdError(snapshotId)
			}
			volumeId = snapshotParts[0] + SEPARATOR + snapshotParts[1]
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
	if len(targetPath) == 0 {
		return nil, fmt.Errorf("given target mount path is empty")
	}
	var volUUID string
	if len(secrets) == 0 {
		klog.Infof("csiNodePublishSecret is  not recieved. Assuming volume given with UUID")
		volUUID = volParts[1]
	} else {
		quobyteClient, err := getAPIClient(secrets, d.ApiURL)
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

	if snapshotName != empty_string {
		// Quobyte snapshots are readonly
		readonly = true
	}

	var options []string
	if readonly {
		options = append(options, "ro")
	}
	volCap := req.GetVolumeCapability()
	if volCap != nil {
		mount := volCap.GetMount()
		if mount != nil {
			mntFlags := mount.GetMountFlags()
			if mntFlags != nil {
				options = append(options, mntFlags...)
			}
		}
	}
	var mountPath string
	if d.IsQuobyteAccesskeysEnabled {
		podUUID := getSanitizedPodUUIDFromPath(targetPath)
		accesskeyID, ok := secrets[accessKeyID]
		if !ok {
			return nil, fmt.Errorf("Mount secret should have '%s: <YOUR_ACCESS_KEY_ID>'", accessKeyID)
		}
		accesskeySecret, ok := secrets["accessKeySecret"]
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
			// TODO (venkat): might need to support tenant|volume|snapshot|subDir
			if len(volParts) == 3 { // tenant|volume|subDir
				// TODO (venkat): add subDir to path volParts[2]
				mountPath = fmt.Sprintf("%s/%s@%s/%s/%s", d.clientMountPoint, accesskeyHandle, volUUID, snapshotsDir, snapshotName)
			} else {
				mountPath = fmt.Sprintf("%s/%s@%s/%s/%s", d.clientMountPoint, accesskeyHandle, volUUID, snapshotsDir, snapshotName)
			}
		}
	} else {
		if snapshotName == empty_string {
			if len(volParts) == 3 { // tenant|volume|subDir
				mountPath = fmt.Sprintf("%s/%s/%s", d.clientMountPoint, volUUID, volParts[2])
			} else {
				mountPath = fmt.Sprintf("%s/%s", d.clientMountPoint, volUUID)
			}
		} else {
			// TODO (venkat): might need to support tenant|volume|snapshot|subDir
			if len(volParts) == 3 { // tenant|volume|subDir
				// TODO (venkat): add subDir to path volParts[2]
				mountPath = fmt.Sprintf("%s/%s/%s/%s", d.clientMountPoint, volUUID, snapshotsDir, snapshotName)
			} else {
				mountPath = fmt.Sprintf("%s/%s/%s/%s", d.clientMountPoint, volUUID, snapshotsDir, snapshotName)
			}
		}
	}
	err := Mount(mountPath, targetPath, "quobyte", options)
	if err != nil {
		return nil, err
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume Currently not implemented as Quobyte has only single mount point
func (d *QuobyteDriver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, fmt.Errorf("target for unmount is empty")
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
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
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
	return nil, status.Errorf(codes.Unimplemented, "NodeGetVolumeStats: Not implented by Quobyte CSI")
}
