package framework

import (
	"fmt"

	"github.com/quobyte/api/v4/quobyte"
)

// The access key types a test can ask CreateAccessKey for. The driver puts whatever is in
// a Secret to two different uses -- management API credentials for the provisioner (see
// src/driver/quobyte_api_client_factory.go) and data access credentials when mounting (see
// src/driver/node.go) -- so a single Secret serving both needs a GeneralAccessKey, while a
// setup with a separate API and mount Secret gets a ManagementAccessKey and a
// DataAccessKey respectively.
const (
	GeneralAccessKey    = quobyte.AccessKeyType_GENERAL_ACCESS_KEY
	ManagementAccessKey = quobyte.AccessKeyType_MANAGEMENT_ACCESS_KEY
	DataAccessKey       = quobyte.AccessKeyType_DATA_ACCESS_KEY
)

// CreateAccessKey creates a non-expiring access key of the given type for userName,
// scoped to tenantID.
func CreateAccessKey(client *quobyte.QuobyteClient, tenantID, userName string, keyType quobyte.AccessKeyType) (quobyte.AccessKeyCredentials, error) {
	resp, err := client.CreateAccessKeyCredentials(&quobyte.CreateAccessKeyCredentialsRequest{
		TenantId:      tenantID,
		UserName:      userName,
		AccessKeyType: keyType,
	})
	if err != nil {
		return quobyte.AccessKeyCredentials{}, fmt.Errorf("creating %s for user %q in tenant %s: %w", keyType, userName, tenantID, err)
	}

	return resp.AccessKeyCredentials, nil
}

// DeleteAccessKey removes an access key created by CreateAccessKey.
func DeleteAccessKey(client *quobyte.QuobyteClient, userName, accessKeyID string) error {
	_, err := client.DeleteAccessKeyCredentials(&quobyte.DeleteAccessKeyCredentialsRequest{
		UserName:    userName,
		AccessKeyId: accessKeyID,
	})
	if err != nil {
		return fmt.Errorf("deleting access key %s of user %q: %w", accessKeyID, userName, err)
	}

	return nil
}
