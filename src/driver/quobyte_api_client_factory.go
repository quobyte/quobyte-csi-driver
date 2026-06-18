package driver

import (
	"fmt"
	"net/url"

	cache "github.com/hashicorp/golang-lru"
	quobyte "github.com/quobyte/api/v4/quobyte"
	"k8s.io/klog"
)

var clientCache *cache.Cache

func init() {
	var err error
	clientCache, err = cache.New(1000)
	if err != nil {
		klog.Fatalf("Could not initialize client cache")
	}
}

//go:generate mockgen -package=mocks -destination  ../mocks/mock_client_provider.go github.com/quobyte/quobyte-csi-driver/driver QuobyteApiClientProvider
type QuobyteApiClientProvider interface {
	NewQuobyteApiClient(ApiURL *url.URL, secrets map[string]string) (quobyte.ExtendedQuobyteApi, error)
}

type QuobyteApiClientFactory struct{}

func (c *QuobyteApiClientFactory) NewQuobyteApiClient(ApiURL *url.URL, secrets map[string]string) (quobyte.ExtendedQuobyteApi, error) {
	var apiUser, apiPass string

	if hasApiAccessKeyIdAndSecret(secrets) {
		apiUser = secrets[accessKeyID]
		apiPass = secrets[accessKeySecret]
	} else if hasApiUserAndPassword(secrets) {
		apiUser = secrets[secretUserKey]
		apiPass = secrets[secretPasswordKey]
	} else {
		return nil, fmt.Errorf("Requires Quobyte management API user/password or accessKeyId/accessKeySecret combination")
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

	apiClient := quobyte.NewQuobyteClient(ApiURL.String(), apiUser, apiPass)
	clientCache.Add(cacheKey, apiClient)

	return apiClient, nil
}
