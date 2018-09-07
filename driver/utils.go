package driver

import (
	"fmt"

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
