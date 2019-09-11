package driver

import (
	"fmt"
	"testing"
)

func TestPodUIDParsing(t *testing.T) {
	expectedUID := "7d40536c8184d7b70e4dc3c27d9acc42b5b32d8e"
	path := fmt.Sprintf("/var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi", expectedUID)
	resultUID := getPodUIDFromPath(path)

	if resultUID != expectedUID {
		t.Errorf("Expected UID: %s but got UID: %s", expectedUID, resultUID)
	}
}

func TestGetAccessKeyValStr(t *testing.T) {
	key_id := "7d40536c8184d7b70e4dc"
	key_secret := "3c27d9acc42b5b32d8e"
	expectedKeyVal := fmt.Sprintf("{\"access_key_id\":\"%s\",\"access_key_secret\":\"%s\"},\"scope\":\"client\"}", key_id, key_secret)
	resultKeyVal := getAccessKeyValStr(key_id, key_secret)

	if resultKeyVal != expectedKeyVal {
		t.Errorf("Expected key value: %s but got key value: %s", expectedKeyVal, resultKeyVal)
	}
}

func TestGetVolUUIDFromErrorMSG(t *testing.T) {
	expectedVolUUID := "7d40536c8184d7b70e4dc3c27d9acc42b5b32d8e"
	errorMsg := fmt.Sprintf("Volume name volumeNameToCheck is already used by volume %s", expectedVolUUID)

	resultUUID := getUUIDFromError(errorMsg)

	if resultUUID != expectedVolUUID {
		t.Errorf("Expected: %s but got: %s", expectedVolUUID, resultUUID)
	}

}
