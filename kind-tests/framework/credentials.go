package framework

import (
	"fmt"
	"testing"

	"github.com/quobyte/api/v4/quobyte"
	corev1 "k8s.io/api/core/v1"
)

// NewCredentialsSecret builds the Secret a test points its StorageClass or PV at, holding
// whichever credentials the driver was deployed to expect: the API user's user/password
// normally, or an access key of that user where access key mounts are on, since the node
// plugin then refuses to mount without accessKeyId/accessKeySecret (src/driver/node.go).
//
// The Secret is only built, not created -- the caller decides where and when. An access
// key, on the other hand, exists in Quobyte the moment this returns, so its revocation is
// registered here, before the caller registers anything of its own: LIFO cleanup then
// revokes the key after whatever used it is gone.
func NewCredentialsSecret(t *testing.T, cfg Config, quobyteClient *quobyte.QuobyteClient, name, tenantID string) *corev1.Secret {
	t.Helper()

	if !cfg.EnableAccessKeyMounts {
		return NewSecret(name, cfg.Namespace, cfg.QuobyteAPIUser, cfg.QuobyteAPIPassword)
	}

	// One key for both uses: this Secret is the provisioner secret and the mount secret
	// at once.
	credentials, err := CreateAccessKey(quobyteClient, tenantID, cfg.QuobyteAPIUser, GeneralAccessKey)
	if err != nil {
		t.Fatalf("creating a Quobyte access key for user %q: %v", cfg.QuobyteAPIUser, err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("quobyte access key %s", credentials.AccessKeyId), func() {
		if err := DeleteAccessKey(quobyteClient, cfg.QuobyteAPIUser, credentials.AccessKeyId); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	return NewAccessKeySecret(name, cfg.Namespace, credentials.AccessKeyId, credentials.SecretAccessKey)
}
