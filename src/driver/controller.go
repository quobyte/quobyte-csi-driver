package driver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	quobyte "github.com/quobyte/api/v4/quobyte"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	VOLUME_HANDLE_PART_SEPARATOR = "|"
	//DefaultCreateQuota Quobyte CSI by default does NOT create volumes with Quotas.
	// To create Quotas for the volumes, set createQuota: "true" in storage class
	DefaultCreateQuota = false
	DefaultAccessModes = 700
	// Permissions for /<shared-volume>
	// Shared volume needs permission to create directory, delete directory in it
	DefaultSharedVolumeAccessModes = 1777
	// Permissions for /<shared-volume>/<pvc-volume>
	// Set sticky bit so that other users cannot delete volume
	// As of golang version 1.22.5, the sticky bit does not work and only user:group:other permissions work
	// https://github.com/golang/go/issues/44575
	DefaultPVCAccessModeInsideSharedVolume = 1700
	// Metadata from K8S CSI external provisioner
	pvcNamespaceKey = "csi.storage.k8s.io/pvc/namespace"
	pinnedKey       = "pinned"
	SnapshotIDKey   = "snapshot_id_key"
	// VolumeHandle prefix for snapshots PV.
	SnapshotVolumeHandlePrefix = "SnapshotVolumeHandle-"
	SharedVolumeNameKey        = "sharedVolumeName"
)

// CreateVolume creates quobyte volume
func (d *QuobyteDriver) CreateVolume(
	ctx context.Context,
	req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if err := validateCreateVolumeRequest(req); err != nil {
		return nil, err
	}

	// if snapshot volume create request, just populate with snapshot id and return.
	// No need to create the volume as Quobyte snapshots does not require a new volume per
	// snapshot
	if result, err := createSnapshotResponse(req); err != nil {
		return nil, err
	} else if result != nil {
		return result, nil
	} // else: not a snapshot volume

	params := req.Parameters
	secrets := req.Secrets
	capacity := req.GetCapacityRange().RequiredBytes
	k8sPVName := req.Name
	volRequest := &quobyte.CreateVolumeRequest{}
	// will be overridden if shared volume name is specified in storage class
	volRequest.Name = k8sPVName
	createQuota := DefaultCreateQuota
	_, isSharedVolume := params[SharedVolumeNameKey]
	volRequest.AccessMode = accessMode(isSharedVolume)

	quobyteClient, err := d.quobyteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
	if err != nil {
		return nil, err
	}
	whoAmIReq := &quobyte.WhoAmIRequest{}
	userInfo, err := quobyteClient.WhoAmI(whoAmIReq)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve user/group via Quobyte API")
	}
	volRequest.RootUserId = userInfo.UserName
	volRequest.RootGroupId = userInfo.PrimaryGroup

	var configuredVolumeAccessMode int32 = 0
	for k, v := range params {
		switch strings.ToLower(k) {
		case "quobytetenant":
			volRequest.TenantId = v
		case "sharedvolumename":
			volRequest.Name = v
		case "user":
			if len(strings.TrimSpace(v)) > 0 {
				volRequest.RootUserId = v
			}
		case "group":
			volRequest.RootGroupId = v
		case "createquota":
			createQuota = strings.ToLower(v) == "true"
		case "labels":
			volRequest.Label, err = parseLabels(v)
			if err != nil {
				return nil, err
			}
		case "accessmode":
			u64, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return nil, err
			}
			configuredVolumeAccessMode = int32(u64)
			if !isSharedVolume {
				volRequest.AccessMode = configuredVolumeAccessMode
			} // else: if shared volume not exists, created with DefaultSharedVolumeAccessModes
		}
	}

	if len(volRequest.RootGroupId) == 0 {
		return nil, fmt.Errorf("primary group is empty. Configure user '%s' primary group or provide override in StorageClass",
			volRequest.RootUserId)
	}

	// Use storage class tenant if provided, otherwise use namespace as tenant if feature is enabled
	if len(volRequest.TenantId) == 0 && d.UseK8SNamespaceAsQuobyteTenant {
		if pvcNamespace, ok := params[pvcNamespaceKey]; ok {
			volRequest.TenantId = pvcNamespace
		} else {
			return nil, fmt.Errorf("To use K8S namespace to Quobyte tenant mapping, " +
				"quay.io/k8scsi/csi-provisioner should be deployed with " +
				"--extra-create-metadata=true. Please redeploy driver with the above flag" +
				" and retry.")
		}
	}

	if len(volRequest.TenantId) > 0 {
		if volRequest.TenantId, err = quobyteClient.GetTenantUUID(volRequest.TenantId); err != nil {
			return nil, err
		}
	}

	var volUUID string

	volCreateResp, err := quobyteClient.CreateVolume(volRequest)
	if err != nil {
		// CSI requires idempotency. (calling volume create multiple times should return the volume if it already exists)
		if !strings.Contains(err.Error(), "ENTITY_EXISTS_ALREADY") {
			return nil, err
		}
		volUUID, err = quobyteClient.ResolveVolumeNameToUUID(volRequest.Name, volRequest.TenantId)
		if err != nil {
			return nil, err
		}
	} else {
		volUUID = volCreateResp.VolumeUuid
	}

	// Creating a new volume/existence of volume alone is not sufficient when Quobyte
	// tenant is configured with "disable_oversubscription: true"
	// Creation of a quota ensures that provisioning succeeds only if there is sufficient Quota
	// available for the request (must enable StorageClass createQuota).
	// For shared volume, do not set Quota, as the requested Quota would be for directory
	// (subpath of shared volume) - quota should be set by admin at tenant level for shared
	// volumes
	if !isSharedVolume && createQuota {
		err := quobyteClient.SetVolumeQuota(volUUID, capacity)
		if err != nil {
			// Volume is just created and volume database is empty. Therefore, use DeleteVolume
			// call to delete the volume database and volume immediately (no need to erase any
			// file data - so avoid erase API call)
			quobyteClient.DeleteVolumeByResolvingNamesToUUID(volUUID, "")
			return nil, err
		}
	}

	var volumeId string
	if isSharedVolume {
		// Apply configured access permissions to the subdirectory
		if configuredVolumeAccessMode == 0 {
			volRequest.AccessMode = DefaultPVCAccessModeInsideSharedVolume
		} else {
			volRequest.AccessMode = configuredVolumeAccessMode
		}
		// requested dynamic volume is subdir under the given shared volume
		subdirPath := filepath.Join(d.clientMountPoint, volUUID, k8sPVName)
		sharedDirectoryMaker := &DirectoryMaker{d.mounter}
		if err := sharedDirectoryMaker.Mkdir(subdirPath, volRequest); err != nil {
			return nil, err
		}
		volumeId = volRequest.TenantId + VOLUME_HANDLE_PART_SEPARATOR + volUUID + VOLUME_HANDLE_PART_SEPARATOR + k8sPVName
	} else {
		volumeId = volRequest.TenantId + VOLUME_HANDLE_PART_SEPARATOR + volUUID
	}
	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeId,
			CapacityBytes: capacity,
		},
	}
	return resp, nil
}

// returns nil, nil if not a snapshot volume creation request
func createSnapshotResponse(req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.GetVolumeContentSource() == nil {
		return nil, nil
	}

	volumeContentSource := req.GetVolumeContentSource()
	snapshot := volumeContentSource.GetSnapshot()
	if snapshot == nil {
		return nil, nil
	}
	volumeContext := make(map[string]string)
	snapshotIdParts := strings.Split(
		snapshot.SnapshotId,
		VOLUME_HANDLE_PART_SEPARATOR)
	// CreateSnapshot should create a valid snapshot id
	if len(snapshotIdParts) < 3 {
		return nil, getInvalidSnapshotIdError(snapshot.SnapshotId)
	}
	volumeContext[SnapshotIDKey] = snapshot.SnapshotId
	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			// k8s expects that storage system takes snapshot (during creation of VolumeSnapshot and VolumeSnapshotContent)
			// and later populates a volume (with its own volumeId) based on the snapshot
			// (during creation of PVC with VolumeSnapshot ref).
			// Used to filter out snapshot based PVs during volume delete.
			// We only create dummy PV for snapshot volumes
			// as Quobyte doesn't create separate volumes for snapshots, there is no need
			// to delete volume/snapshot with PV. Deletion of VolumeSnapshot and VolumeSnapshotContent
			// should delete the snapshot.
			VolumeId:      SnapshotVolumeHandlePrefix + req.Name,
			CapacityBytes: req.CapacityRange.RequiredBytes,
			ContentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: snapshot.SnapshotId,
					},
				},
			},
			VolumeContext: volumeContext,
		},
	}
	return resp, nil
}

func accessMode(isSharedVolume bool) int32 {
	if isSharedVolume {
		return DefaultSharedVolumeAccessModes
	}
	return DefaultAccessModes
}

// DeleteVolume deletes the given volume or
// the directory inside a shared volume
func (d *QuobyteDriver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volID := req.GetVolumeId()
	if len(volID) == 0 {
		return nil, fmt.Errorf("volumeId is required for DeleteVolume")
	}

	if strings.HasPrefix(volID, SnapshotVolumeHandlePrefix) {
		// Snapshot volume and hence the PV being deleted is a dummy volume
		// See CreateVolume for more information
		return &csi.DeleteVolumeResponse{}, nil
	}
	volumeIdParts := strings.Split(volID, VOLUME_HANDLE_PART_SEPARATOR)
	if len(volumeIdParts) < 2 || len(volumeIdParts) > 3 {
		return nil, fmt.Errorf("given volumeHandle '%s' is not in the form <Tenant_Name/Tenant_UUID>%s<VOL_NAME/VOL_UUID>[|<subdir>]", volID, VOLUME_HANDLE_PART_SEPARATOR)
	}
	secrets := req.GetSecrets()
	if len(secrets) == 0 {
		return nil, fmt.Errorf("secrets are required to delete a volume." +
			" Provide csi.storage.k8s.io/provisioner-secret-name/namespace in storage class")
	}
	quobyteClient, err := d.quobyteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
	if err != nil {
		return nil, err
	}

	if len(volumeIdParts) == 2 { // tenant|volume
		err = quobyteClient.EraseVolumeByResolvingNamesToUUID(volumeIdParts[1], volumeIdParts[0], d.ImmediateErase)
	} else if len(volumeIdParts) == 3 { // tenant|volume|subdir
		if !d.UseDeleteFilesTask {
			subdirPath := filepath.Join(d.clientMountPoint, volumeIdParts[1], volumeIdParts[2])
			renameTo := filepath.Join(d.clientMountPoint, volumeIdParts[1], fmt.Sprintf(DELETE_MARKER_FORMAT, d.Name, volumeIdParts[2]))
			if err := d.mounter.Rename(subdirPath, renameTo); err != nil {
				if e, ok := err.(*os.LinkError); ok && e.Err != syscall.ENOENT {
					return nil, fmt.Errorf("Cannot mark directory '%s' for deletion due to %s", subdirPath, err)
				}
			}
		} else {
			req := &quobyte.CreateTaskRequest{}
			req.RestrictToVolumes = []string{volumeIdParts[1]}
			req.TaskType = quobyte.TaskType_DELETE_FILES_IN_VOLUMES
			req.DeleteFilesSettings = quobyte.DeleteFilesSettings{}
			req.DeleteFilesSettings.DirectoryPath = "/" + volumeIdParts[2]
			_, err = quobyteClient.CreateTask(req)
			if err != nil {
				return nil, fmt.Errorf("could not delete subdirectory of the shared volume due to %s", err)
			}
		}
	}

	if err != nil {
		return nil, err
	}
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume  Nothing to do - mounts volume on NodePublishVolume
func (d *QuobyteDriver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	// Quobyte client mounts the volume if it exists
	return &csi.ControllerPublishVolumeResponse{}, nil
}

// ControllerGetVolume - nothing to do
func (d *QuobyteDriver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return &csi.ControllerGetVolumeResponse{}, nil
}

// ControllerUnpublishVolume - nothing to do
func (d *QuobyteDriver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	// Quobyte does not require any clean up, return to the Quobyte client
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities Quobyte CSI does not implement this method.
func (d *QuobyteDriver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ValidateVolumeCapabilities: Not implemented by Quobyte CSI")
}

// ListVolumes Quobyte CSI does not implement this method.
func (d *QuobyteDriver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ListVolumes: Not implemented by Quobyte CSI")
}

// GetCapacity Quobyte volumes are not capacity bound by default
func (d *QuobyteDriver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	// TODO (venkat) : This seems to be the storage system capacity query and not of the volume
	return nil, status.Errorf(codes.Unimplemented, "GetCapacity: Not implemented by Quobyte CSI")
}

// ControllerGetCapabilities returns supported capabilities.
// CREATE_DELETE_VOLUME is required but
// PUBLISH_UNPUBLISH_VOLUME not required since Quobyte Client does the volume attachments to the node.
func (d *QuobyteDriver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	newCap := func(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
	}

	var caps []*csi.ControllerServiceCapability
	for _, cap := range []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	} {
		caps = append(caps, newCap(cap))
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}

	return resp, nil
}

func (d *QuobyteDriver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	var isPinned bool
	if pinned, ok := req.Parameters[pinnedKey]; ok {
		pinnedVal, err := strconv.ParseBool(pinned)
		if err != nil {
			return nil, fmt.Errorf("VolumeSnapshotClass.Parameters.pinned must be true/false. Configured value %s is invalid.", pinned)
		}
		isPinned = pinnedVal
	} else {
		isPinned = false
	}
	volumeId := req.SourceVolumeId
	volParts := strings.Split(volumeId, VOLUME_HANDLE_PART_SEPARATOR)
	if len(volParts) < 2 {
		return nil, fmt.Errorf("given volumeId %s is not of the form <Tenant>%s<Volume>", volumeId, VOLUME_HANDLE_PART_SEPARATOR)
	}
	secrets := req.Secrets
	quobyteClient, err := d.quobyteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
	if err != nil {
		return nil, err
	}
	volUUID, err := quobyteClient.GetVolumeUUID(volParts[1], volParts[0])
	if err != nil {
		return nil, err
	}
	// Append tenant to make it available for delete snapshot calls.
	// Dynamic provision always resolves (tenant/volume) name to UUID
	// For pre-provisioned volume/snapshot, customer can configure either
	// (tenant/volume) name/uuid, for this reason we need to resolve tenant/volume to UUID
	// combination of tenant, volume and snapshot name
	tenantUUID, err := quobyteClient.GetTenantUUID(volParts[0])
	if err != nil {
		return nil, err
	}

	snapshotReq := &quobyte.CreateSnapshotRequest{VolumeUuid: volUUID, Name: req.Name, Pinned: isPinned}
	_, err = quobyteClient.CreateSnapshot(snapshotReq)
	if err != nil {
		// CSI requires idempotency. (calling snapshot create multiple times should return the snapshot if it already exists)
		if !strings.Contains(err.Error(), "ENTITY_EXISTS_ALREADY/POSIX_ERROR_NONE") {
			return nil, err
		}
	}
	// TODO(venkat): Pre-provisioned PV can have subdir, so append subDir to the snapshotId
	snapshotID := tenantUUID + VOLUME_HANDLE_PART_SEPARATOR + volUUID + VOLUME_HANDLE_PART_SEPARATOR + req.Name
	timestamp := &timestamp.Timestamp{Seconds: time.Now().Unix()}
	resp := &csi.CreateSnapshotResponse{Snapshot: &csi.Snapshot{SnapshotId: snapshotID, SourceVolumeId: req.SourceVolumeId, CreationTime: timestamp, ReadyToUse: true}}
	return resp, nil
}

func (d *QuobyteDriver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	snapshotID := req.SnapshotId
	snapshotParts := strings.Split(snapshotID, VOLUME_HANDLE_PART_SEPARATOR)
	if len(snapshotParts) < 3 {
		return nil, fmt.Errorf("invalid snapshot UID: %s. VolumeSnapshotRef.uid must be of form '<tenant>%s<volume>%s<snapshot-name>'",
			snapshotID, VOLUME_HANDLE_PART_SEPARATOR, VOLUME_HANDLE_PART_SEPARATOR)
	}
	secrets := req.Secrets
	quobyteClient, err := d.quobyteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
	if err != nil {
		return nil, err
	}
	tenantUUID, err := quobyteClient.GetTenantUUID(snapshotParts[0])
	if err != nil {
		return nil, err
	}
	volUUID, err := quobyteClient.GetVolumeUUID(snapshotParts[1], tenantUUID)
	if err != nil {
		return nil, err
	}
	snapshotDeleteReq := &quobyte.DeleteSnapshotRequest{VolumeUuid: volUUID, Name: snapshotParts[2]}
	_, err = quobyteClient.DeleteSnapshot(snapshotDeleteReq)
	if err != nil {
		return nil, err
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (d *QuobyteDriver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	snapshotID := req.SnapshotId
	snapshotParts := strings.Split(snapshotID, VOLUME_HANDLE_PART_SEPARATOR)
	if len(snapshotParts) < 3 {
		return nil, fmt.Errorf("invalid snapshot UID: %s. VolumeSnapshotRef.uid must be of form '<tenant>%s<volume>%s<snapshot-name>'",
			snapshotID, VOLUME_HANDLE_PART_SEPARATOR, VOLUME_HANDLE_PART_SEPARATOR)
	}
	secrets := req.Secrets
	quobyteClient, err := d.quobyteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
	if err != nil {
		return nil, err
	}
	tenantUUID, err := quobyteClient.GetTenantUUID(snapshotParts[0])
	if err != nil {
		return nil, err
	}
	volUUID, err := quobyteClient.GetVolumeUUID(snapshotParts[1], tenantUUID)
	if err != nil {
		return nil, err
	}

	listReq := &quobyte.ListSnapshotsRequest{VolumeUuid: volUUID}
	listResp, err := quobyteClient.ListSnapshots(listReq)
	if err != nil {
		return nil, err
	}
	snapshotEntries := make([]*csi.ListSnapshotsResponse_Entry, len(listResp.Snapshot))
	for i, entry := range listResp.Snapshot {
		// important we use tenant and volume from req.SnapshotId
		// to match the snapshot id
		snapshotID := snapshotParts[0] + VOLUME_HANDLE_PART_SEPARATOR + snapshotParts[1] + VOLUME_HANDLE_PART_SEPARATOR + entry.Name
		snapshotEntries[i] = &csi.ListSnapshotsResponse_Entry{Snapshot: &csi.Snapshot{SourceVolumeId: entry.VolumeUuid, SnapshotId: snapshotID, CreationTime: &timestamp.Timestamp{Seconds: (entry.Timestamp / 1000)}, ReadyToUse: true}}
	}
	return &csi.ListSnapshotsResponse{Entries: snapshotEntries}, nil
}

func (d *QuobyteDriver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if req.GetCapacityRange() == nil {
		return nil, fmt.Errorf("invalid capacity range nil for expand volume")
	}
	capacity := req.CapacityRange.RequiredBytes
	if err := d.expandVolume(req.VolumeId, req.Secrets, capacity); err != nil {
		return nil, err
	}
	return &csi.ControllerExpandVolumeResponse{CapacityBytes: capacity}, nil
}

func (d *QuobyteDriver) expandVolume(volumeId string, secrets map[string]string, capacity int64) error {
	volParts := strings.Split(volumeId, VOLUME_HANDLE_PART_SEPARATOR)
	if len(volParts) < 2 {
		return fmt.Errorf("given volumeHandle '%s' is not in the form <Tenant_Name/Tenant_UUID>|<VOL_NAME/VOL_UUID>", volumeId)
	}
	// Shared volume is assumed to have unlimited capacity. If need user should set
	// Quota limits on via Quobyte management API/webconsole
	// We return success if expansion is requested. This gives customer flexibility with PVC
	// rescaling on k8s. On the other hand, failed status requires destruction
	// of pod, pvc, pv and recreation to rescale PVC.
	if len(volParts) == 3 {
		return nil
	}
	if len(secrets) == 0 {
		return fmt.Errorf("controller-expand-secret-name and controller-expand-secret-namespace should be configured")
	}
	quobyteClient, err := d.quobyteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
	if err != nil {
		return err
	}
	volUUID, err := quobyteClient.GetVolumeUUID(volParts[1], volParts[0])
	if err != nil {
		return err
	}
	err = quobyteClient.SetVolumeQuota(volUUID, capacity)
	if err != nil {
		return err
	}
	return nil
}

func (d *QuobyteDriver) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ControllerModifyVolume: Not implemented by Quobyte CSI")
}
