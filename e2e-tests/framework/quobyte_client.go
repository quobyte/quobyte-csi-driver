package framework

import (
	"github.com/quobyte/api/v4/quobyte"
)

// NewQuobyteClient builds a Quobyte API client from cfg's credentials, using
// the same client library the CSI driver itself uses
// (see src/driver/quobyte_api_client_factory.go).
func NewQuobyteClient(cfg Config) *quobyte.QuobyteClient {
	return quobyte.NewQuobyteClient(cfg.QuobyteAPIURL, cfg.QuobyteAPIUser, cfg.QuobyteAPIPassword)
}
