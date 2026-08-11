package framework

import (
	"errors"
	"fmt"
	"strings"

	"github.com/quobyte/api/v4/quobyte"
)

// SharedVolumeAccessMode is the POSIX access mode a shared volume's root directory needs
// so the driver can create and remove a subdirectory per PVC in it. Mirrors
// DefaultSharedVolumeAccessModes in src/driver/controller.go, which is what the driver
// itself uses when it creates the shared volume.
const SharedVolumeAccessMode int32 = 1777

// CreateSharedVolume creates the Quobyte volume a StorageClass's sharedVolumeName points
// at, the way the driver would create it on the first provisioning request: owned by the
// API user this client is authenticated as, with an access mode that lets the driver
// manage subdirectories in it. Returns the volume's UUID.
//
// Idempotent: a volume of that name already in the tenant is resolved and returned
// instead, the same way the driver handles ENTITY_EXISTS_ALREADY.
func CreateSharedVolume(client *quobyte.QuobyteClient, name, tenantID string) (string, error) {
	// The driver resolves the owner the same way, so a pre-created volume ends up
	// indistinguishable from one the driver made.
	userInfo, err := client.WhoAmI(&quobyte.WhoAmIRequest{})
	if err != nil {
		return "", fmt.Errorf("resolving the Quobyte API user to own shared volume %q: %w", name, err)
	}

	createResp, err := client.CreateVolume(&quobyte.CreateVolumeRequest{
		Name:        name,
		TenantId:    tenantID,
		RootUserId:  userInfo.UserName,
		RootGroupId: userInfo.PrimaryGroup,
		AccessMode:  SharedVolumeAccessMode,
	})
	if err == nil {
		return createResp.VolumeUuid, nil
	}
	if !strings.Contains(err.Error(), "ENTITY_EXISTS_ALREADY") {
		return "", fmt.Errorf("creating shared volume %q in tenant %s: %w", name, tenantID, err)
	}

	volumeUUID, err := client.ResolveVolumeNameToUUID(name, tenantID)
	if err != nil {
		return "", fmt.Errorf("resolving already existing shared volume %q in tenant %s: %w", name, tenantID, err)
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
