package driver

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewQuobyteApiClient(t *testing.T) {
	assert := assert.New(t)
	apiUrl, err := url.Parse("http://dummy.quobyte.api")

	secrets := map[string]string{
		secretUserKey: "someUser",
	}
	clientFactory := &QuobyteApiClientFactory{}
	client, err := clientFactory.NewQuobyteApiClient(apiUrl, secrets)
	assert.NotNil(err)
	assert.Nil(client)

	secrets = map[string]string{
		secretUserKey:   "someUser",
		accessKeySecret: "someAccessKeySecret",
	}
	clientFactory = &QuobyteApiClientFactory{}
	client, err = clientFactory.NewQuobyteApiClient(apiUrl, secrets)
	assert.NotNil(err)
	assert.Nil(client)

	secrets = map[string]string{}
	clientFactory = &QuobyteApiClientFactory{}
	client, err = clientFactory.NewQuobyteApiClient(apiUrl, secrets)
	assert.NotNil(err)
	assert.Nil(client)

	secrets = map[string]string{
		secretUserKey:     "someUser",
		secretPasswordKey: "somePassword",
	}

	clientFactory = &QuobyteApiClientFactory{}
	client, err = clientFactory.NewQuobyteApiClient(apiUrl, secrets)
	assert.Nil(err)
	assert.NotNil(client)

	secrets = map[string]string{
		secretUserKey:     "someUser",
		secretPasswordKey: "somePassword",
	}

	clientFactory = &QuobyteApiClientFactory{}
	client, err = clientFactory.NewQuobyteApiClient(apiUrl, secrets)
	assert.Nil(err)
	assert.NotNil(client)

	secrets = map[string]string{
		accessKeyID:     "someAccessKeyId",
		accessKeySecret: "someAccessKeySecret",
	}
	clientFactory = &QuobyteApiClientFactory{}
	client, err = clientFactory.NewQuobyteApiClient(apiUrl, secrets)
	assert.Nil(err)
	assert.NotNil(client)
}
