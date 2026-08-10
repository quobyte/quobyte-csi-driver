package framework

import (
	"fmt"

	"github.com/quobyte/api/v4/quobyte"
)

// CreateAccessKey creates a non-expiring access key for userName scoped to
// tenantID. The key is a GENERAL_ACCESS_KEY because the Secret it ends up in is
// used for both purposes at once: as management API credentials by the
// provisioner (see src/driver/quobyte_api_client_factory.go) and as data access
// credentials when mounting (see src/driver/node.go).
func CreateAccessKey(client *quobyte.QuobyteClient, tenantID, userName string) (quobyte.AccessKeyCredentials, error) {
	resp, err := client.CreateAccessKeyCredentials(&quobyte.CreateAccessKeyCredentialsRequest{
		TenantId:      tenantID,
		UserName:      userName,
		AccessKeyType: quobyte.AccessKeyType_GENERAL_ACCESS_KEY,
	})
	if err != nil {
		return quobyte.AccessKeyCredentials{}, fmt.Errorf("creating access key for user %q in tenant %s: %w", userName, tenantID, err)
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
