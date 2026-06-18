package driver

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/quobyte/api/v4/quobyte"
	"github.com/stretchr/testify/assert"
)

func TestGetVolUUIDFromErrorMSG(t *testing.T) {
	expectedVolUUID := "7d40536c8184d7b70e4dc3c27d9acc42b5b32d8e"
	errorMsg := fmt.Sprintf("Volume name volumeNameToCheck is already used by volume %s", expectedVolUUID)

	resultUUID := getUUIDFromError(errorMsg)

	if resultUUID != expectedVolUUID {
		t.Errorf("Expected: %s but got: %s", expectedVolUUID, resultUUID)
	}
}

func TestQuobyteApiClientSecretsCheck(t *testing.T) {
	var secrets = make(map[string]string)

	check := hasApiCredentials(secrets)

	if check {
		t.Errorf("expected: false got: %t", check)
	}

	secrets[secretUserKey] = "dummyUser"
	secrets[secretPasswordKey] = "dummyPassword"

	check = hasApiUserAndPassword(secrets)

	if !check {
		t.Errorf("expected: true got: %t", check)
	}

	check = hasApiCredentials(secrets)
	if !check {
		t.Errorf("expected: true got: %t", check)
	}

	secrets = make(map[string]string)
	secrets[accessKeyID] = "dummyAccessKeyId"
	secrets[accessKeySecret] = "dummyAccessKeySecret"

	check = hasApiAccessKeyIdAndSecret(secrets)
	if !check {
		t.Errorf("expected: true got: %t", check)
	}
}

func TestParseLabels(t *testing.T) {
	assert := assert.New(t)

	gotLabels, err := parseLabels("label:")
	assert.NotNil(err)

	gotLabels, err = parseLabels("label")
	assert.NotNil(err)

	gotLabels, err = parseLabels(":")
	assert.NotNil(err)

	gotLabels, err = parseLabels("label:  ")
	assert.NotNil(err)

	gotLabels, err = parseLabels(": value")
	assert.NotNil(err)

	gotLabels, err = parseLabels("label:value")
	wantedLabels := []*quobyte.Label{
		{
			Name:  "label",
			Value: "value",
		},
	}
	assert.Nil(err)
	assert.True(reflect.DeepEqual(wantedLabels, gotLabels))

	gotLabels, err = parseLabels("label:value, label2:value2")
	wantedLabels = []*quobyte.Label{
		{
			Name:  "label",
			Value: "value",
		},
		{
			Name:  "label2",
			Value: "value2",
		},
	}
	assert.Nil(err)
	assert.True(reflect.DeepEqual(wantedLabels, gotLabels))

	// Allow spaces in
	gotLabels, err = parseLabels("label : value   , label2 :value2 ")
	wantedLabels = []*quobyte.Label{
		{
			Name:  "label",
			Value: "value",
		},
		{
			Name:  "label2",
			Value: "value2",
		},
	}
	assert.Nil(err)
	assert.True(reflect.DeepEqual(wantedLabels, gotLabels))
}

func TestValidateVolumeCapabilities(t *testing.T) {
	assert := assert.New(t)
	err := validateCreateVolumeRequest(nil)
	assert.NotNil(err)

	req := &csi.CreateVolumeRequest{
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
		Parameters:    map[string]string{},
		Secrets:       map[string]string{"a": "b"},
	}
	req.VolumeCapabilities = []*csi.VolumeCapability{}
	err = validateCreateVolumeRequest(req)
	assert.Nil(err)

	// Quobyte CSI Driver does not support block volumes
	req.VolumeCapabilities = []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Block{
				Block: &csi.VolumeCapability_BlockVolume{},
			},
		},
	}
	err = validateCreateVolumeRequest(req)
	assert.NotNil(err)
}
