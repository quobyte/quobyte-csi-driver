package driver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	quobyte "github.com/quobyte/api/quobyte"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	SEPARATOR = "|"
	//DefaultTenant Default Tenant to use if none provided by user
	DefaultTenant = "My Tenant"
	//DefaultConfig Default configuration to use if none provided by user
	DefaultConfig = "BASE"
	//DefaultCreateQuota Quobyte CSI by default does NOT create volumes with Quotas.
	// To create Quotas for the volumes, set createQuota: "true" in storage class
	DefaultCreateQuota = false
	DefaultUser        = "root"
	DefaultGroup       = "nfsnobody"
	DefaultAccessModes = 777
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
	volName := req.Name
	volRequest := &quobyte.CreateVolumeRequest{}
	volRequest.Name = volName
	volRequest.TenantId = DefaultTenant
	volRequest.ConfigurationName = DefaultConfig
	volRequest.RootUserId = DefaultUser
	volRequest.RootGroupId = DefaultGroup
	createQuota := DefaultCreateQuota
	volRequest.AccessMode = DefaultAccessModes
	for k, v := range params {
		switch strings.ToLower(k) {
		case "quobytetenant":
			volRequest.TenantId = v
		case "user":
			volRequest.RootUserId = v
		case "group":
			volRequest.RootGroupId = v
		case "quobyteconfig":
			volRequest.ConfigurationName = v
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
			volRequest.AccessMode = int32(u64)
		}
	}
	quobyteClient, err := getAPIClient(secrets, d.ApiURL)
	if err != nil {
		return nil, err
	}

	if d.UseK8SNamespaceAsQuobyteTenant {
		if pvcNamespace, ok := params[pvcNamespaceKey]; ok {
			volRequest.TenantId = pvcNamespace
		} else {
			return nil, fmt.Errorf("To use K8S namespace to Quobyte tenant mapping, quay.io/k8scsi/csi-provisioner" +
				"should be deployed with --extra-create-metadata=true. Please redeploy driver with the above flag and retry.")
		}
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
				return nil, getInvlaidSnapshotIdError(snapshot.SnapshotId)
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

	volCreateResp, err := quobyteClient.CreateVolume(volRequest)
	var volUUID string
	if err != nil {
		// CSI requires idempotency. (calling volume create multiple times should return the volume if it already exists)
		if !strings.Contains(err.Error(), "ENTITY_EXISTS_ALREADY/POSIX_ERROR_NONE") {
			return nil, err
		}
		volUUID = getUUIDFromError(fmt.Sprintf("%v", err))
	} else {
		volUUID = volCreateResp.VolumeUuid
	}
	if createQuota {
		err := quobyteClient.SetVolumeQuota(volUUID, capacity)
		if err != nil {
			req := &quobyte.DeleteVolumeRequest{VolumeUuid: volUUID}
			quobyteClient.DeleteVolume(req)
			return nil, err
		}
	}
	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			// CSI does not pass on vendor specific parameters to DeleteVolume and we require API url during volume delete
			// this hacky append serves the purpose as of now. The format of the hack <TenantName/TenantUUID>|<VOL_NAME/VOLUME_UUID>
			// Implications of this are
			// 	 1. All the subsequent calls should not use value of req.GetVolumeId() or req.VolumeId directly as volume name
			//   but parse and resolve UUID to name wherever required.
			//   2. Must be aware of the  <TenantName/TenantUUID>|<VOL_NAME/VOLUME_UUID> while using req.GetVolumeId() or req.VolumeId

			// Currently volume handle is the combination of  <TenantName/TenantUUID>, and <VOL_NAME/VOLUME_UUID>
			// due to the limitation of CSI not passing storage vendor specific parameters. Dynamic provision used UUID returned by
			// Quobyte's CreateVolume call as it does not require name to UUID resolution calls. But user can configure either name or UUID
			// for pre-provisioned volumes
			VolumeId:      volRequest.TenantId + SEPARATOR + volUUID,
			CapacityBytes: capacity,
		},
	}
	return resp, nil
}

// DeleteVolume deletes the given volume.
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
		return nil, fmt.Errorf("secrets are required delete volume." +
			" Provide csi.storage.k8s.io/provisioner-secret-name/namespace in storage class")
	}
	params := strings.Split(volID, SEPARATOR)
	if len(params) < 2 {
		return nil, fmt.Errorf("given volumeHandle '%s' is not in the form <Tenant_Name/Tenant_UUID>%s<VOL_NAME/VOL_UUID>", volID, SEPARATOR)
	}
	quobyteClient, err := getAPIClient(secrets, d.ApiURL)
	if err != nil {
		return nil, err
	}
	err = quobyteClient.DeleteVolumeByResolvingNamesToUUID(params[1], params[0])
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
			return nil, fmt.Errorf("VolumeSnapshotClass.Parameters.pinned must be ture/false. Configured value %s is invalid.", pinned)
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
	quobyteClient, err := getAPIClient(secrets, d.ApiURL)
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
	quobyteClient, err := getAPIClient(secrets, d.ApiURL)
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
	quobyteClient, err := getAPIClient(secrets, d.ApiURL)
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
	d.expandVolume(&ExpandVolumeReq{volID: req.VolumeId, expandSecrets: req.Secrets, capacity: capacity})
	return &csi.ControllerExpandVolumeResponse{CapacityBytes: capacity}, nil
}
