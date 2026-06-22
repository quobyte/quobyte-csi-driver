package driver

import (
	"errors"
	"fmt"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	quobyte "github.com/quobyte/api/v4/quobyte"
)

var (
	KEY_VAL          = "{ \"access_key_id\": \"%s\",\"access_key_secret\": \"%s\",\"access_context\": \"%s\",\"access_key_scope\": \"context\" }"
	VOL_UUID_LOCATOR = "used by volume "
	POD_UUID_LOCATOR = "/pods/"
	POD_VOL_LOCATOR  = "/volume"
)

const (
	secretUserKey     string = "user"
	secretPasswordKey string = "password"
	accessKeyID       string = "accessKeyId"
	accessKeySecret   string = "accessKeySecret"
)

func getAccessKeyValStr(key_id, key_secret, accessKeyHandle string) string {
	return fmt.Sprintf(KEY_VAL, key_id, key_secret, accessKeyHandle)
}

func getUUIDFromError(str string) string {
	index := strings.Index(str, VOL_UUID_LOCATOR)
	uuid := str[index+len(VOL_UUID_LOCATOR):]
	return strings.TrimSpace(uuid)
}

func validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	if req == nil {
		return errors.New("invalid create volume request")
	}
	if req.GetCapacityRange() == nil {
		return errors.New("capacity range must not be empty")
	}
	if req.Parameters == nil {
		return errors.New("request must not be empty")
	}
	if len(req.Secrets) == 0 {
		return fmt.Errorf("secrets are required to dynamically provision a volume. " +
			"Provide csi.storage.k8s.io/provisioner-secret-<name/namespace> in storage class")
	}
	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil
	}
	for _, cap := range caps {
		if cap.GetBlock() != nil {
			return errors.New("Quobyte CSI provisioner does not support block volumes.")
		}
	}
	return nil
}

func parseLabels(labels string) ([]*quobyte.Label, error) {
	labelKVs := strings.Split(labels, ",")
	parsedLabels := make([]*quobyte.Label, 0)
	for _, labelKV := range labelKVs {
		labelKV = strings.TrimSpace(labelKV)
		labelKVArr := strings.Split(labelKV, ":")
		if len(labelKVArr) < 2 {
			return nil, fmt.Errorf("Found invalid label '%s'. Label should be of the form <Name>:<Value>.", labelKV)
		}
		key := strings.TrimSpace(labelKVArr[0])
		value := strings.TrimSpace(labelKVArr[1])
		if len(key) == 0 {
			return nil, fmt.Errorf("Found invalid label '%s'. Label name must not be empty.", labelKV)
		}
		if len(value) == 0 {
			return nil, fmt.Errorf("Found invalid label '%s'. Label value must not be empty.", labelKV)
		}
		label := &quobyte.Label{
			Name:  key,
			Value: value,
		}
		parsedLabels = append(parsedLabels, label)
	}
	return parsedLabels, nil
}

func getInvalidSnapshotIdError(snapshotId string) error {
	return fmt.Errorf("given snapshot id %s is not of the form <Tenant>%s<Volume>%s<Snapshot_Name>[%sSubDirectory]",
		snapshotId, VOLUME_HANDLE_PART_SEPARATOR,
		VOLUME_HANDLE_PART_SEPARATOR, VOLUME_HANDLE_PART_SEPARATOR)
}

func hasApiCredentials(secrets map[string]string) bool {
	return hasApiUserAndPassword(secrets) || hasApiAccessKeyIdAndSecret(secrets)
}

func hasApiAccessKeyIdAndSecret(secrets map[string]string) bool {
	_, hasQuobyteApiKeyId := secrets[accessKeyID]
	_, hasQuobyteApiSecret := secrets[accessKeySecret]
	return hasQuobyteApiKeyId && hasQuobyteApiSecret
}

func hasApiUserAndPassword(secrets map[string]string) bool {
	_, hasQuobyteApiUser := secrets[secretUserKey]
	_, hasQuobyteApiPassword := secrets[secretPasswordKey]
	return hasQuobyteApiUser && hasQuobyteApiPassword
}
