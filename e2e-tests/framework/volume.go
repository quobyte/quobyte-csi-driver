package framework

import (
	"errors"
	"fmt"
	"strings"

	"github.com/quobyte/api/v4/quobyte"
)

// The POSIX access modes the driver gives a volume's root directory, mirroring
// DefaultAccessModes and DefaultSharedVolumeAccessModes in src/driver/controller.go, so a
// volume a test pre-creates is indistinguishable from one the driver made. A shared volume
// needs the wider mode because the driver creates and removes a subdirectory per PVC in it.
const (
	DefaultVolumeAccessMode int32 = 700
	SharedVolumeAccessMode  int32 = 1777
)

// CreateSharedVolume creates the Quobyte volume a StorageClass's sharedVolumeName points
// at, the way the driver would create it on the first provisioning request.
func CreateSharedVolume(client *quobyte.QuobyteClient, name, tenantID string) (string, error) {
	return CreateVolume(client, name, tenantID, SharedVolumeAccessMode)
}

// CreateVolume creates a Quobyte volume owned by the API user this client is
// authenticated as, and returns its UUID. Where the volume has to be usable through the
// credentials of the test's own user -- anything not world-writable -- use
// CreateVolumeOwnedBy instead.
//
// Idempotent: a volume of that name already in the tenant is resolved and returned
// instead, the same way the driver handles ENTITY_EXISTS_ALREADY.
func CreateVolume(client *quobyte.QuobyteClient, name, tenantID string, accessMode int32) (string, error) {
	// The driver resolves the owner the same way, so a pre-created volume ends up
	// indistinguishable from one the driver made.
	userInfo, err := client.WhoAmI(&quobyte.WhoAmIRequest{})
	if err != nil {
		return "", fmt.Errorf("resolving the Quobyte API user to own volume %q: %w", name, err)
	}

	return CreateVolumeOwnedBy(client, name, tenantID, accessMode, userInfo.UserName, userInfo.PrimaryGroup)
}

// CreateVolumeOwnedBy creates a Quobyte volume owned by the given user and group, and
// returns its UUID. A test that pre-creates a volume and then mounts it through its own
// user's credentials (framework.CreateTestUser) has to own it that way: the default access
// mode of a volume, 700, admits nobody else.
//
// Idempotent in the same way as CreateVolume.
func CreateVolumeOwnedBy(client *quobyte.QuobyteClient, name, tenantID string, accessMode int32, ownerUser, ownerGroup string) (string, error) {
	createResp, err := client.CreateVolume(&quobyte.CreateVolumeRequest{
		Name:        name,
		TenantId:    tenantID,
		RootUserId:  ownerUser,
		RootGroupId: ownerGroup,
		AccessMode:  accessMode,
	})
	if err == nil {
		return createResp.VolumeUuid, nil
	}
	if !strings.Contains(err.Error(), "ENTITY_EXISTS_ALREADY") {
		return "", fmt.Errorf("creating volume %q in tenant %s: %w", name, tenantID, err)
	}

	volumeUUID, err := client.ResolveVolumeNameToUUID(name, tenantID)
	if err != nil {
		return "", fmt.Errorf("resolving already existing volume %q in tenant %s: %w", name, tenantID, err)
	}

	return volumeUUID, nil
}

// DeleteVolume removes the Quobyte volume with the given UUID.
//
// Deliberately the DeleteVolume API and not the Erase variant: the CSI driver erases
// volumes on DeleteVolume (see src/driver/controller.go), which only *schedules* the
// erasure, so a volume can still be listed in its tenant well after the PV it backed is
// gone. Tests that have to know a volume is really gone -- before deleting the tenant
// holding it, say -- delete it outright instead. A volume that is already gone is not an
// error, so this is safe to call on a volume the driver may or may not have removed.
func DeleteVolume(client *quobyte.QuobyteClient, volumeUUID string) error {
	listResp, err := client.GetVolumeList(&quobyte.GetVolumeListRequest{
		VolumeUuid: []string{volumeUUID},
	})
	if err != nil {
		return fmt.Errorf("looking up volume %s: %w", volumeUUID, err)
	}
	if len(listResp.Volume) == 0 {
		return nil
	}

	// TODO(venkat): Update the quobyte API and force delete the volume
	if _, err := client.DeleteVolume(&quobyte.DeleteVolumeRequest{VolumeUuid: volumeUUID}); err != nil {
		return fmt.Errorf("deleting volume %s: %w", volumeUUID, err)
	}

	return nil
}

// TenantOfVolume returns the UUID of the tenant the given volume lives in, asking Quobyte
// rather than the Kubernetes objects.
//
// Needed because a PV's volume handle does not always say. The driver builds it as
// "<tenant>|<volume>" out of the tenant *it* resolved (CreateVolume in
// src/driver/controller.go), and where neither the StorageClass nor the namespace mapping
// names one it sends no tenant at all and lets the Quobyte API pick from the credentials --
// so the handle starts with an empty first part and the volume is the only way back to the
// tenant that was chosen.
//
// Whatever the API reports the volume's tenant as, name or UUID, comes back as a UUID:
// GetTenantUUID passes a UUID through and resolves anything else.
func TenantOfVolume(client *quobyte.QuobyteClient, volumeUUID string) (string, error) {
	listResp, err := client.GetVolumeList(&quobyte.GetVolumeListRequest{
		VolumeUuid: []string{volumeUUID},
	})
	if err != nil {
		return "", fmt.Errorf("looking up volume %s to find its tenant: %w", volumeUUID, err)
	}

	for _, volume := range listResp.Volume {
		if volume == nil || volume.VolumeUuid != volumeUUID {
			continue
		}
		tenantID, err := client.GetTenantUUID(volume.TenantDomain)
		if err != nil {
			return "", fmt.Errorf("resolving tenant %q of volume %s: %w", volume.TenantDomain, volumeUUID, err)
		}
		if tenantID == "" {
			return "", fmt.Errorf("the Quobyte API reports no tenant for volume %s", volumeUUID)
		}

		return tenantID, nil
	}

	return "", fmt.Errorf("volume %s not found, so it has no tenant to report", volumeUUID)
}

// ListVolumeNamesInTenant returns the names of the volumes currently in the given tenant,
// for tests that need to show the driver did (or did not) create a volume of its own.
func ListVolumeNamesInTenant(client *quobyte.QuobyteClient, tenantID string) ([]string, error) {
	listResp, err := client.GetVolumeList(&quobyte.GetVolumeListRequest{TenantDomain: tenantID})
	if err != nil {
		return nil, fmt.Errorf("listing volumes of tenant %s: %w", tenantID, err)
	}

	names := make([]string, 0, len(listResp.Volume))
	for _, volume := range listResp.Volume {
		if volume == nil {
			continue
		}
		names = append(names, volume.Name)
	}

	return names, nil
}

// GetVolumeQuota returns the logical disk space quota set on the given volume, in bytes,
// and whether there is one at all. The driver sets it through SetVolumeQuota on
// provisioning (only with createQuota) and again on expansion, so this is what tells a test
// that a StorageClass parameter or a PVC resize reached the storage system rather than only
// the Kubernetes objects.
//
// A volume with no quota is not an error: createQuota defaults to false in the driver
// (DefaultCreateQuota in src/driver/controller.go), and "no quota was set" is exactly what
// some of the tests here assert.
func GetVolumeQuota(client *quobyte.QuobyteClient, volumeUUID string) (int64, bool, error) {
	resp, err := client.GetQuota(&quobyte.GetQuotaRequest{
		OnlyEntity: []*quobyte.ConsumingEntity{
			{
				Type:       quobyte.ConsumingEntity_Type_VOLUME,
				Identifier: volumeUUID,
			},
		},
		OnlyResourceType: []*quobyte.Resource_Type{&logicalDiskSpace},
	})
	if err != nil {
		return 0, false, fmt.Errorf("reading the quota of volume %s: %w", volumeUUID, err)
	}

	for _, quota := range resp.Quotas {
		if quota == nil {
			continue
		}
		for _, limit := range quota.Limits {
			if limit != nil && limit.Type == quobyte.Resource_Type_LOGICAL_DISK_SPACE {
				return limit.Value, true, nil
			}
		}
	}

	return 0, false, nil
}

// Addressable because GetQuotaRequest.OnlyResourceType is a slice of pointers.
var logicalDiskSpace = quobyte.Resource_Type_LOGICAL_DISK_SPACE

// CapTenantDiskSpace puts a logical disk space quota on the tenant and forbids
// oversubscribing it, so that a volume quota larger than what is left has to be refused.
//
// This is how a test provokes the one failure the driver has an error path for: with
// createQuota in the StorageClass, CreateVolume sets a quota right after creating the volume
// and deletes the volume again if that fails (src/driver/controller.go). Without a tenant
// limit the API would happily accept any volume quota, oversubscribed or not, and that path
// would never be reached.
//
// Only for a tenant the test created: it replaces whatever quota the tenant has.
func CapTenantDiskSpace(client *quobyte.QuobyteClient, tenantID string, bytes int64) error {
	_, err := client.SetQuota(&quobyte.SetQuotaRequest{
		Quotas: []*quobyte.Quota{
			{
				Consumer: []*quobyte.ConsumingEntity{
					{
						Type:                    quobyte.ConsumingEntity_Type_TENANT,
						Identifier:              tenantID,
						DisableOversubscription: true,
					},
				},
				Limits: []*quobyte.Resource{
					{
						Type:  quobyte.Resource_Type_LOGICAL_DISK_SPACE,
						Value: bytes,
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("limiting tenant %s to %d bytes of logical disk space: %w", tenantID, bytes, err)
	}

	return nil
}

// GetVolumeLabels returns the labels set on the given volume, as name -> value. The driver
// sets them from the StorageClass's "labels" parameter when it creates the volume (see
// parseLabels in src/driver/utils.go), and nothing else here does, so this is what shows
// that parameter reached Quobyte.
func GetVolumeLabels(client *quobyte.QuobyteClient, volumeUUID string) (map[string]string, error) {
	resp, err := client.GetLabels(&quobyte.GetLabelsRequest{
		FilterEntityType: quobyte.Label_EntityType_VOLUME,
		FilterEntityId:   volumeUUID,
	})
	if err != nil {
		return nil, fmt.Errorf("reading the labels of volume %s: %w", volumeUUID, err)
	}

	labels := make(map[string]string, len(resp.Label))
	for _, label := range resp.Label {
		if label == nil {
			continue
		}
		labels[label.Name] = label.Value
	}

	return labels, nil
}

// DeleteVolumesInTenant removes every volume still listed in the given tenant and returns
// how many it deleted. Used to empty a tenant a test created before deleting the tenant
// itself -- a tenant that still holds volumes cannot be deleted, and the driver's erase
// leaves volumes behind for a while (see DeleteVolume).
//
// Only ever call this for a tenant the test itself created: it deletes everything in the
// tenant, which would be destructive against a pre-existing one.
func DeleteVolumesInTenant(client *quobyte.QuobyteClient, tenantID string) (int, error) {
	listResp, err := client.GetVolumeList(&quobyte.GetVolumeListRequest{TenantDomain: tenantID})
	if err != nil {
		return 0, fmt.Errorf("listing volumes of tenant %s: %w", tenantID, err)
	}

	deleted := 0
	var errs []error
	for _, volume := range listResp.Volume {
		if volume == nil {
			continue
		}
		if err := DeleteVolume(client, volume.VolumeUuid); err != nil {
			errs = append(errs, err)
			continue
		}
		deleted++
	}

	return deleted, errors.Join(errs...)
}
