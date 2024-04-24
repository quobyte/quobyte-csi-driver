package driver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	quobyte "github.com/quobyte/api/quobyte"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	SEPARATOR = "|"
	//DefaultConfig Default configuration to use if none provided by user
	DefaultConfig = "BASE"
	//DefaultCreateQuota Quobyte CSI by default does NOT create volumes with Quotas.
	// To create Quotas for the volumes, set createQuota: "true" in storage class
	DefaultCreateQuota = false
	DefaultUser        = "root"
	DefaultGroup       = "nfsnobody"
	DefaultAccessModes = 700
	// Metadata from K8S CSI external provisioner
	pvcNamespaceKey = "csi.storage.k8s.io/pvc/namespace"
	pinnedKey       = "pinned"
	SnapshotIDKey   = "snapshot_id_key"
	// VolumeHandle prefix for snapshots PV.
	SnapshotVolumeHandlePrefix = "SnapshotVolumeHandle-"
)

// CreateVolume creates quobyte volume
func (d *QuobyteDriver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("container orchestrator should send the storage cluster details")
	}
	err := validateVolCapabilities(req.GetVolumeCapabilities())
	if err != nil {
		return nil, err
	}
	params := req.Parameters
	secrets := req.Secrets
	if len(secrets) == 0 {
		return nil, fmt.Errorf("secrets are required to dynamically provision volume." +
			"Provide csi.storage.k8s.io/provisioner-secret-name/namespace in storage class")
	}

	capacity := req.GetCapacityRange().RequiredBytes
	dynamicVolumeName := req.Name
	volRequest := &quobyte.CreateVolumeRequest{}
	// will be overriden if shared volume name is specified in storage class
	volRequest.Name = dynamicVolumeName
	volRequest.ConfigurationName = DefaultConfig
	volRequest.RootUserId = DefaultUser
	volRequest.RootGroupId = DefaultGroup
	createQuota := DefaultCreateQuota
	volRequest.AccessMode = DefaultAccessModes
	for k, v := range params {
		switch strings.ToLower(k) {
		case "quobytetenant":
			volRequest.TenantId = v
		case "sharedvolumename":
			volRequest.Name = v
		case "user":
			volRequest.RootUserId = v
		case "group":
			volRequest.RootGroupId = v
		case "quobyteconfig":
			volRequest.ConfigurationName = v
		case "createquota":
			createQuota = strings.ToLower(v) == "true"
		case "labels":
			if d.QuobyteVersion == 3 {
				volRequest.Label, err = parseLabels(v)
				if err != nil {
					return nil, err
				}
			}
		case "accessmode":
			u64, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return nil, err
			}
			volRequest.AccessMode = int32(u64)
		}
	}

	quobyteClient, err := quoybteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
	if err != nil {
		return nil, err
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

	if len(volRequest.TenantId) == 0 {
		return nil, fmt.Errorf("Configure quobyteTenant in StorageClass parameters or deploy" +
			" driver with useK8SNamespaceAsTenant feature enabled")
	}

	volRequest.TenantId, err = quobyteClient.GetTenantUUID(volRequest.TenantId)
	if err != nil {
		return nil, err
	}

	// if snapshot request, just populate with snapshot id and return.
	// No need to create the volume as volume already created before
	volumeContext := make(map[string]string)
	volumeContentSource := req.GetVolumeContentSource()
	if volumeContentSource != nil {
		snapshot := volumeContentSource.GetSnapshot()
		if snapshot != nil {
			snapshotIdParts := strings.Split(snapshot.SnapshotId, SEPARATOR)
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
					CapacityBytes: capacity,
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
	}

	var volUUID string

	volCreateResp, err := quobyteClient.CreateVolume(volRequest)
	if err != nil {
		// CSI requires idempotency. (calling volume create multiple times should return the volume if it already exists)
		if !strings.Contains(err.Error(), "ENTITY_EXISTS_ALREADY/POSIX_ERROR_NONE") {
			return nil, err
		}
		volUUID, err = quobyteClient.ResolveVolumeNameToUUID(volRequest.Name, volRequest.TenantId)
		if err != nil {
			return nil, err
		}
	} else {
		volUUID = volCreateResp.VolumeUuid
	}

	_, isSharedVolume := params["sharedVolumeName"]
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
		// requested dynamic volume is subdir under the given shared volume
		subdirPath := filepath.Join(d.clientMountPoint, volUUID, dynamicVolumeName)
		if statInfo, err := os.Stat(subdirPath); err != nil {
			if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOENT {
				if err = d.createDynamicVolumeAsADirectory(subdirPath, volRequest); err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		} else if !statInfo.IsDir() {
			return nil, fmt.Errorf("A file with sub-directory name exists at %s", subdirPath)
		}
		volumeId = volRequest.TenantId + SEPARATOR + volUUID + SEPARATOR + dynamicVolumeName
	} else {
		volumeId = volRequest.TenantId + SEPARATOR + volUUID
	}
	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeId,
			CapacityBytes: capacity,
		},
	}
	return resp, nil
}

func (d *QuobyteDriver) createDynamicVolumeAsADirectory(subdirPath string, volRequest *quobyte.CreateVolumeRequest) error {
	modeVal, err := strconv.ParseUint(strconv.Itoa(int(volRequest.AccessMode)), 8, 32)
	if err != nil {
		return fmt.Errorf("Cannot parse access mode due to %s", err)
	}
	if err = os.Mkdir(subdirPath, fs.FileMode(modeVal)); err != nil {
		// ignore directory exists error; might have been created by replicated pods
		if e, ok := err.(*os.PathError); ok && e.Err == syscall.EEXIST {
			// assume user, group and permissions are already set on the existing directory
			return nil
		} else {
			return fmt.Errorf("Unable to create sub-directory %s due to %s", subdirPath, err)
		}
	}
	if err = os.Chmod(subdirPath, fs.FileMode(modeVal)); err != nil {
		return fmt.Errorf("Cannot apply requested permissions %d for %s due to %s", modeVal, subdirPath, err)
	}
	return d.chownDirectory(subdirPath, volRequest)
}

func (d *QuobyteDriver) chownDirectory(subdirPath string, volRequest *quobyte.CreateVolumeRequest) error {
	var userInfo *user.User
	var groupInfo *user.Group
	var err error
	if userInfo, err = user.Lookup(volRequest.RootUserId); err != nil {
		return fmt.Errorf("Cannot look up user '%s' on node '%s' due to error %s", volRequest.RootUserId, d.NodeName, err)
	}
	if groupInfo, err = user.LookupGroup(volRequest.RootGroupId); err != nil {
		return fmt.Errorf("Cannot look up group '%s' on node '%s' due to error %s", volRequest.RootGroupId, d.NodeName, err)
	}
	var userId, groupId int
	if userId, err = strconv.Atoi(userInfo.Uid); err != nil {
		return err
	}
	if groupId, err = strconv.Atoi(groupInfo.Gid); err != nil {
		return err
	}
	if err := os.Chown(subdirPath, userId, groupId); err != nil {
		return fmt.Errorf("Cannot change ownership of '%s' to '%s:%s' on node '%s' due to %s", subdirPath, volRequest.RootUserId, volRequest.RootGroupId, d.NodeName, err)
	}
	return nil
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

	secrets := req.GetSecrets()
	if len(secrets) == 0 {
		return nil, fmt.Errorf("secrets are required to delete a volume." +
			" Provide csi.storage.k8s.io/provisioner-secret-name/namespace in storage class")
	}
	volumeIdParts := strings.Split(volID, SEPARATOR)
	if len(volumeIdParts) < 2 {
		return nil, fmt.Errorf("given volumeHandle '%s' is not in the form <Tenant_Name/Tenant_UUID>%s<VOL_NAME/VOL_UUID>", volID, SEPARATOR)
	}
	quobyteClient, err := quoybteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
	if err != nil {
		return nil, err
	}

	if len(volumeIdParts) == 2 { // tenant|volume
		if d.QuobyteVersion == 2 {
			err = quobyteClient.EraseVolumeByResolvingNamesToUUID_2X(volumeIdParts[1], volumeIdParts[0])
		} else {
			err = quobyteClient.EraseVolumeByResolvingNamesToUUID(volumeIdParts[1], volumeIdParts[0], d.ImmediateErase)
		}
	} else if len(volumeIdParts) == 3 { // tenant|volume|subdir
		if d.QuobyteVersion == 2 {
			subdirPath := filepath.Join(d.clientMountPoint, volumeIdParts[1], volumeIdParts[2])
			renameTo := filepath.Join(d.clientMountPoint, volumeIdParts[1], fmt.Sprintf(DELETE_MARKER_FORMAT, d.NodeName, volumeIdParts[2]))
			if err := os.Rename(subdirPath, renameTo); err != nil {
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
	} else {
		return nil, fmt.Errorf("Unknown volume id format")
	}

	if err != nil {
		return nil, err
	}
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume Quobyte CSI does not implement this method. Quobyte Client is responsible for attaching volume.
func (d *QuobyteDriver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	// Quobyte client mounts the volume if it exists
	return &csi.ControllerPublishVolumeResponse{}, nil
}

// ControllerGetVolume Quobyte CSI does not implement this method.
func (d *QuobyteDriver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return &csi.ControllerGetVolumeResponse{}, nil
}

// ControllerUnpublishVolume Quobyte CSI does not implement this method.
func (d *QuobyteDriver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	// Quobyte does not require any clean up, return to the Quobyte client
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities Quobyte CSI does not implement this method.
func (d *QuobyteDriver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ValidateVolumeCapabilities: Not implented by Quobyte CSI")
}

// ListVolumes Quobyte CSI does not implement this method.
func (d *QuobyteDriver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ListVolumes: Not implented by Quobyte CSI")
}

// GetCapacity Quobyte volumes are not capacity bound by default
func (d *QuobyteDriver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	// TODO (venkat) : This seems to be the storage system capacity query and not of the volume
	return nil, status.Errorf(codes.Unimplemented, "GetCapacity: Quobyte  does not support it, at the moment.")
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
		//	csi.ControllerServiceCapability_RPC_GET_CAPACITY,
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
	volParts := strings.Split(volumeId, SEPARATOR)
	if len(volParts) < 2 {
		return nil, fmt.Errorf("given volumeId %s is not of the form <Tenant>%s<Volume>", volumeId, SEPARATOR)
	}
	secrets := req.Secrets
	quobyteClient, err := quoybteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
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
	snapshotID := tenantUUID + SEPARATOR + volUUID + SEPARATOR + req.Name
	timestamp := &timestamp.Timestamp{Seconds: time.Now().Unix()}
	resp := &csi.CreateSnapshotResponse{Snapshot: &csi.Snapshot{SnapshotId: snapshotID, SourceVolumeId: req.SourceVolumeId, CreationTime: timestamp, ReadyToUse: true}}
	return resp, nil
}

func (d *QuobyteDriver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	snapshotID := req.SnapshotId
	snapshotParts := strings.Split(snapshotID, SEPARATOR)
	if len(snapshotParts) < 3 {
		return nil, fmt.Errorf("invalid snapshot UID: %s. VolumeSnapshotRef.uid must be of form '<tenant>%s<volume>%s<snapshot-name>'",
			snapshotID, SEPARATOR, SEPARATOR)
	}
	secrets := req.Secrets
	quobyteClient, err := quoybteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
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
	snapshotParts := strings.Split(snapshotID, SEPARATOR)
	if len(snapshotParts) < 3 {
		return nil, fmt.Errorf("invalid snapshot UID: %s. VolumeSnapshotRef.uid must be of form '<tenant>%s<volume>%s<snapshot-name>'",
			snapshotID, SEPARATOR, SEPARATOR)
	}
	secrets := req.Secrets
	quobyteClient, err := quoybteClientFactory.NewQuobyteApiClient(d.ApiURL, secrets)
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
		snapshotID := snapshotParts[0] + SEPARATOR + snapshotParts[1] + SEPARATOR + entry.Name
		snapshotEntries[i] = &csi.ListSnapshotsResponse_Entry{Snapshot: &csi.Snapshot{SourceVolumeId: entry.VolumeUuid, SnapshotId: snapshotID, CreationTime: &timestamp.Timestamp{Seconds: (entry.Timestamp / 1000)}, ReadyToUse: true}}
	}
	return &csi.ListSnapshotsResponse{Entries: snapshotEntries}, nil
}

func (d *QuobyteDriver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	capacity := req.CapacityRange.RequiredBytes
	if err := d.expandVolume(&ExpandVolumeReq{volID: req.VolumeId, expandSecrets: req.Secrets, capacity: capacity}); err != nil {
		return nil, err
	}
	return &csi.ControllerExpandVolumeResponse{CapacityBytes: capacity}, nil
}

func (d *QuobyteDriver) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ControllerModifyVolume: Not implented by Quobyte CSI")
}
