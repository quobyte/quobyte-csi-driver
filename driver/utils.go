package driver

import (
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	quobyte "github.com/quobyte/api"
)

func getAPIClient(secrets map[string]string, apiURL string) (*quobyte.QuobyteClient, error) {
	var apiUser, apiPass string
	var ok bool

	if apiUser, ok = secrets["user"]; !ok {
		return nil, fmt.Errorf("Quobyte API user missing in secret")
	}

	if apiPass, ok = secrets["password"]; !ok {
		return nil, fmt.Errorf("Quobyte API password missing in secret")
	}

	return quobyte.NewQuobyteClient(apiURL, apiUser, apiPass), nil
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func (d *QuobyteDriver) expandVolume(req *ExpandVolumeReq) error {
	volID := req.volID
	volParts := strings.Split(volID, "|")
	if len(volParts) < 2 {
		return fmt.Errorf("given volumeHandle '%s' is not in the form <Tenant_Name/Tenant_UUID>|<VOL_NAME/VOL_UUID>", volID)
	}
	secrets := req.expandSecrets
	if len(secrets) == 0 {
		return fmt.Errorf("controller-expand-secret-name and controller-expand-secret-namespace should be configured")
	}
	quobyteClient, err := getAPIClient(secrets, d.ApiURL)
	capacity := req.capacity
	volUUID, err := quobyteClient.GetVolumeUUID(volParts[1], volParts[0])
	if err != nil {
		return err
	}
	err = quobyteClient.SetVolumeQuota(volUUID, int64(capacity))
	if err != nil {
		return err
	}
	return nil
}

func getUUIDFromError(errMsg string) string {
	uuidLocator := "used by volume "
	index := strings.Index(errMsg, uuidLocator)
	uuid := errMsg[index+len(uuidLocator) : len(errMsg)-2]
	return strings.TrimSpace(uuid)
}

func validateVolCapabilities(caps []*csi.VolumeCapability) error {
	for _, cap := range caps {
		if cap.GetBlock() != nil {
			return fmt.Errorf("Quobyte CSI provisioner does not support block volumes.")
		}
	}
	return nil
}
