package driver

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	quobyte "github.com/quobyte/api"
)

var (
	KEY_VAL          = "{\"access_key_id\":\"%s\",\"access_key_secret\":\"%s\"},\"scope\":\"client\"}"
	VOL_UUID_LOCATOR = "used by volume "
	POD_UUID_LOCATOR = "/pods/"
	POD_VOL_LOCATOR  = "/volume"
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

func getAccessKeyValStr(key_id, key_secret string) string {
	return fmt.Sprintf(KEY_VAL, key_id, key_secret)
}

func setfattr(key, val, mountPath string) error {
	cmd := "setfattr"
	var options []string
	options = append(options, "-n")
	options = append(options, key)
	options = append(options, "-v")
	options = append(options, val)
	options = append(options, mountPath)
	if out, err := exec.Command(cmd, options...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed setfattr: %v cmd: '%s %s' command output: %q", err, cmd, options, string(out))
	}
	return nil
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
	err = quobyteClient.SetVolumeQuota(volUUID, uint64(capacity))
	if err != nil {
		return err
	}
	return nil
}

func getUUIDFromError(str string) string {
	index := strings.Index(str, VOL_UUID_LOCATOR)
	uuid := str[index+len(VOL_UUID_LOCATOR) : len(str)]
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

func getPodUIDFromPath(podVolPath string) string {
	// Extracts the Pod UID from the given pod volume path. Path of pod volume is of the
	// form /var/lib/kubelet/pods/<THE-POD-ID-HERE>/volumes/kubernetes.io~csi
	pod_uid_start_index := strings.Index(podVolPath, POD_UUID_LOCATOR) + len(POD_UUID_LOCATOR)
	pod_uid_end_index := strings.Index(podVolPath, POD_VOL_LOCATOR)
	return podVolPath[pod_uid_start_index:pod_uid_end_index]
}
