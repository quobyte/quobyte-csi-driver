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
	xAttrKey                        string      = "quobyte.access_key"
	snapshotsDir                    string      = ".snapshots"
	accessKeyContextHandleSeparator string      = "@"
	mountPathDefaultPermissions     os.FileMode = 0750
)

// NodePublishVolume mounts the volume to the pod with the given target path
// QuobyteClient does the mounting of the volumes
func (d *QuobyteDriver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	var volumeHandle string

	var snapshotName string
	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, fmt.Errorf("given target mount path is empty")
	}
	if err := d.mounter.Mkdirs(targetPath, mountPathDefaultPermissions); err != nil {
		return nil, err
	}
	// see controller.go -- CreateVolume method. VolumeContext is only added for snapshot volumes
	volumeHandle, snapshotName, err := mayGetVolumeHandleFromSnapshotContext(req)
	if err != nil {
		return nil, err
	}
	// In case of pre-provisioned volumes, NodePublishSecrets are not taken from storage class but
	// needs to be passed as nodePublishSecretRef in PV (kubernetes) definition
	secrets := req.GetSecrets()

	tenant, volume, subDirectory, err := processVolumeHandle(volumeHandle)
	if err != nil {
		return nil, err
	}
	volumeUUID := volume
	if len(secrets) == 0 || !hasApiCredentials(secrets) {
		// cannot resolve volume Id without Quobyte API credentials if tenant name & volume name is given..assume volume uuid
		klog.Infof("csiNodePublishSecret is  not received with sufficient Quobyte API credential. Assuming volume given with UUID")
	} else {
		quobyteClient, err := d.quobyteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
		if err != nil {
			return nil, err
		}
		// Even if secrets are present, they may be Quobyte mount/file system secrets which
		// cannot be used against Quobyte API. So, the request may fail.
		// volume name should be retrieved from the req.GetVolumeId()
		// Due to csi lacking in parameter passing during delete Volume, req.volumeId is changed
		// to <TENANT_NAME/TENANT_UUID>|<VOL_NAME/VOL_UUID>. see controller.go CreateVolume for the details.
		volumeUUID, err = quobyteClient.GetVolumeUUID(volume, tenant)
		if err != nil {
			return nil, err
		}
	}

	options := getMountOptions(req)
	// https://kubernetes.io/docs/concepts/storage/storage-classes/#mount-options
	if len(options) > 0 {
		return nil, fmt.Errorf("Mount options are not supported by Quobyte CSI Driver but provided %d options", len(options))
	}
	var mountPath string
	var accessKeyHandle string
	if d.IsQuobyteAccessKeyMountsEnabled {
		accessKeyHandle = d.mounter.CreateAccessKeyContextHandle()
		keyID, ok := secrets[accessKeyID]
		if !ok {
			return nil, fmt.Errorf("Mount secret should have '%s: <YOUR_ACCESS_KEY_ID>'", accessKeyID)
		}
		keySecret, ok := secrets[accessKeySecret]
		if !ok {
			return nil, fmt.Errorf("Mount secret should have '%s: <YOUR_ACCESS_KEY_SECRET>'", accessKeySecret)
		}
		xAttrVal := getAccessKeyValStr(keyID, keySecret, accessKeyHandle)
		// In case of setfattr failure:
		// - Make sure Quobyte CSI driver is deployed with "enableAccessKeyMounts: true"
		// - Quobyte clients are deployed with access key flags enabled - see "Requirements" section of
		// https://github.com/quobyte/quobyte-csi-driver/blob/master/docs/quobyte_access_keys.md
		if err := d.mounter.SetXAttr(xAttrKey, xAttrVal, d.clientMountPoint); err != nil {
			klog.Errorf("failed setting access key handle as an extended attribute due to %v", err)
			return nil, err
		}
	}
	mountPath = formMountPath(d.clientMountPoint, volumeUUID, accessKeyHandle, snapshotName, subDirectory)
	if err := Mount(mountPath, targetPath, d.mounter); err != nil {
		return nil, err
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func mayGetVolumeHandleFromSnapshotContext(req *csi.NodePublishVolumeRequest) (string, string, error) {
	if !strings.HasPrefix(req.VolumeId, SnapshotVolumeHandlePrefix) {
		return req.GetVolumeId(), "" /* no snapshot */, nil
	}
	volumeContext := req.GetVolumeContext()
	if volumeContext == nil {
		return "", "", fmt.Errorf("volume context should not empty for snapshot")
	}
	snapshotHandle, ok := volumeContext[SnapshotIDKey]
	if !ok {
		return "", "", fmt.Errorf("%s key is not found in the volume context", SnapshotIDKey)
	}
	snapshotParts := strings.Split(snapshotHandle, VOLUME_HANDLE_PART_SEPARATOR)
	if len(snapshotParts) < 3 {
		return "", "", getInvalidSnapshotIdError(snapshotHandle)
	}
	var volumeHandle string
	var snapshotName = snapshotParts[2]
	if len(snapshotParts) == 4 {
		// Ex: volumeHandle = tenant|volume|subDirectory
		volumeHandle = snapshotParts[0] + VOLUME_HANDLE_PART_SEPARATOR + snapshotParts[1] + VOLUME_HANDLE_PART_SEPARATOR + snapshotParts[3]
	} else {
		// Ex: volumeHandle = tenant|volume
		volumeHandle = snapshotParts[0] + VOLUME_HANDLE_PART_SEPARATOR + snapshotParts[1]
	}
	return volumeHandle, snapshotName, nil
}

func formMountPath(clientMountPoint, volumeUUID, accessKeyHandle, snapshotName, subDirectory string) string {
	if len(accessKeyHandle) > 0 {
		volumeUUID = accessKeyHandle + accessKeyContextHandleSeparator + volumeUUID
	}
	// basic volume mount path - /home/clientMount/volumeUuid
	mountPath := fmt.Sprintf("%s/%s", clientMountPoint, volumeUUID)
	if len(snapshotName) > 0 {
		// /home/clientMount/volumeUuid/.snapshots/mySnapshot
		mountPath = fmt.Sprintf("%s/%s/%s", mountPath, snapshotsDir, snapshotName)
	}
	if len(subDirectory) > 0 {
		// /home/clientMount/volumeUuid/.snapshots/mySnapshot/mySubDir or
		// /home/clientMount/volumeUuid/mySubDir
		mountPath = fmt.Sprintf("%s/%s", mountPath, subDirectory)
	}
	return mountPath
}

func getMountOptions(req *csi.NodePublishVolumeRequest) []string {
	volCap := req.GetVolumeCapability()
	if volCap == nil || volCap.GetMount() == nil {
		return nil
	}
	return volCap.GetMount().GetMountFlags()
}

func processVolumeHandle(volumeHandle string) (string, string, string, error) {
	volumeHandleFragments := strings.Split(volumeHandle, VOLUME_HANDLE_PART_SEPARATOR)
	if len(volumeHandleFragments) < 2 {
		return "", "", "", fmt.Errorf("given volumeHandle '%s' is not in the format <TENANT_NAME/TENANT_UUID>%s<VOL_NAME/VOL_UUID>", volumeHandle, VOLUME_HANDLE_PART_SEPARATOR)
	}
	if len(volumeHandleFragments) == 2 {
		return volumeHandleFragments[0], volumeHandleFragments[1], "", nil
	}
	return volumeHandleFragments[0], volumeHandleFragments[1], volumeHandleFragments[2], nil
}

// NodeUnpublishVolume Currently not implemented as Quobyte has only single mount point
func (d *QuobyteDriver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, fmt.Errorf("target path for unmount is empty")
	}
	klog.Infof("Unmounting %s", target)
	err := Unmount(target, d.mounter)
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

// NodeStageVolume Unimplemented - not required for Quobyte CSI Driver
func (d *QuobyteDriver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "NodeStageVolume: Not implemented by Quobyte CSI")
}

// NodeUnstageVolume Unimplemented - not required for Quobyte CSI Driver
func (d *QuobyteDriver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "NodeUnstageVolume: Not implemented by Quobyte CSI")
}

func (d *QuobyteDriver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: d.NodeName,
	}, nil
}

func (d *QuobyteDriver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "NodeExpandVolume: Not implemented by Quobyte CSI")
}

func (d *QuobyteDriver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if !d.enabledVolumeMetrics {
		return nil, fmt.Errorf("volume/PVC metrics export is disabled for the Quobyte CSI Driver %s", d.Name)
	}
	volumePath := req.GetVolumePath()
	if len(volumePath) <= 0 {
		return nil, fmt.Errorf("volume path must not be empty")
	}
	var statfs unix.Statfs_t
	var err error
	if statfs, err = d.mounter.Statfs(volumePath); err != nil {
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
