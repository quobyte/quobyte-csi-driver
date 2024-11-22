package driver

import (
	"fmt"
	"testing"
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
