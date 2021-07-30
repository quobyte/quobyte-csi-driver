package driver

import (
	"fmt"
	"k8s.io/klog"
	"net/url"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	cache "github.com/hashicorp/golang-lru"
	quobyte "github.com/quobyte/api/quobyte"
)

var (
	KEY_VAL          = "{ \"access_key_id\": \"%s\",\"access_key_secret\": \"%s\",\"access_context\": \"%s\",\"access_key_scope\": \"context\" }"
	VOL_UUID_LOCATOR = "used by volume "
	POD_UUID_LOCATOR = "/pods/"
	POD_VOL_LOCATOR  = "/volume"
)

var clientCache *cache.Cache = nil

func initClientCache() {
	if clientCache == nil {
		var err error
		clientCache, err = cache.New(1000)
		if err != nil {
			klog.Fatalf("Could not initialize client cache")
		}
	}
}

func getAPIClient(secrets map[string]string, apiURL *url.URL) (*quobyte.QuobyteClient, error) {
	if clientCache == nil {
		initClientCache()
	}
	var apiUser, apiPass string
	var ok bool

	if apiUser, ok = secrets["user"]; !ok {
		return nil, fmt.Errorf("Quobyte API user missing in secret")
	}

	if apiPass, ok = secrets["password"]; !ok {
		return nil, fmt.Errorf("Quobyte API password missing in secret")
	}

	// API url is unique for deployment and cannot be changed once driver is installed.
	// Therefore, it is not need as part of key.
	// Add password to key to create a new client if the password for the user is changed.
	cacheKey := apiUser + apiPass

	if apiClientIf, ok := clientCache.Get(cacheKey); ok {
		if apiClient, ok := apiClientIf.(*quobyte.QuobyteClient); ok {
			return apiClient, nil
		}
		return nil, fmt.Errorf("Cached API client is not QuobyteClient type")
	}

	apiClient := quobyte.NewQuobyteClient(apiURL.String(), apiUser, apiPass)
	clientCache.Add(cacheKey, apiClient)

	return apiClient, nil
}

func getAccessKeyValStr(key_id, key_secret, accesskeyHandle string) string {
	return fmt.Sprintf(KEY_VAL, key_id, key_secret, accesskeyHandle)
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
		return fmt.Errorf("failed setfattr due to %v. command output: %q", err, string(out))
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
	err = quobyteClient.SetVolumeQuota(volUUID, capacity)
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

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func getSanitizedPodUUIDFromPath(podVolPath string) string {
	// Extracts the Pod UID from the given pod volume path. Path of pod volume is of the
	// form /var/lib/kubelet/pods/<THE-POD-ID-HERE>/volumes/kubernetes.io~csi
	pod_uid_start_index := strings.Index(podVolPath, POD_UUID_LOCATOR) + len(POD_UUID_LOCATOR)
	pod_uid_end_index := strings.Index(podVolPath, POD_VOL_LOCATOR)
	return strings.ReplaceAll(podVolPath[pod_uid_start_index:pod_uid_end_index], "-", "")
}

func parseLabels(labels string) ([]*quobyte.Label, error) {
	labelKVs := strings.Split(labels, ",")
	parsedLabels := make([]*quobyte.Label, 0)
	for _, lableKV := range labelKVs {
		labelKVArr := strings.Split(lableKV, ":")
		if len(labelKVArr) < 2 {
			return parsedLabels, fmt.Errorf("Found invalid label '%s'. Label should be <Name>:<Value>", lableKV)
		}
		label := &quobyte.Label{
			Name:  labelKVArr[0],
			Value: labelKVArr[1],
		}
		parsedLabels = append(parsedLabels, label)
	}
	return parsedLabels, nil
}

func getInvlaidSnapshotIdError(snapshotId string) error {
	return fmt.Errorf("given snapshot id %s is not of the form <Tenant>%s<Volume>%s<Snapshot_Name>", snapshotId, SEPARATOR, SEPARATOR)
}
