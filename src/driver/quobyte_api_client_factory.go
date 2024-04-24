package driver

import (
	"fmt"
	"net/url"

	cache "github.com/hashicorp/golang-lru"
	quobyte "github.com/quobyte/api/quobyte"
	"k8s.io/klog"
)

var quoybteClientFactory QuobyteApiClientFactory
var clientCache *cache.Cache

func init() {
	var err error
	clientCache, err = cache.New(1000)
	if err != nil {
		klog.Fatalf("Could not initialize client cache")
	}

	quoybteClientFactory = QuobyteApiClientFactory{}
}

type QuobyteApiClientProvider interface {
	NewQuobyteApiClient(secrets map[string]string) (*quobyte.QuobyteClient, error)
}

type QuobyteApiClientFactory struct{}

func (c *QuobyteApiClientFactory) NewQuobyteApiClient(ApiURL *url.URL, secrets map[string]string) (*quobyte.QuobyteClient, error) {
	var apiUser, apiPass string

	// TODO (venkat): priority to access key after 2.x support EOL
	if hasApiUserAndPassword(secrets) { // Quobyte API access using user and password
		apiUser = secrets[secretUserKey]
		apiPass = secrets[secretPasswordKey]
	} else if hasApiAccessKeyIdAndSecret(secrets) { // Quobyte API access using access key & secret
		apiUser = secrets[accessKeyID]
		apiPass = secrets[accessKeySecret]
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
