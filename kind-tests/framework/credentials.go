package framework

import (
	"fmt"
	"testing"
	"time"

	"github.com/quobyte/api/v4/quobyte"
	corev1 "k8s.io/api/core/v1"
)

// TestUser is the Quobyte user a test provisions through, created for that test alone by
// CreateTestUser. PrimaryGroup is part of it because a volume the test pre-creates has to
// be owned by this user/group pair to be usable through the credentials below.
type TestUser struct {
	Name         string
	Password     string
	PrimaryGroup string
}

// NewCredentialsSecret builds the Secret a test points its StorageClass or PV at. See
// NewCredentials, which it wraps for the tests that need nothing but the Secret.
func NewCredentialsSecret(t *testing.T, cfg Config, quobyteClient *quobyte.QuobyteClient, name string, tenantIDs ...string) *corev1.Secret {
	t.Helper()

	_, secret := NewCredentials(t, cfg, quobyteClient, name, tenantIDs...)

	return secret
}

// NewCredentials creates the Quobyte user a test provisions through and builds the Secret
// holding its credentials: the user's user/password normally, or an access key of that user
// where access key mounts are on, since the node plugin then refuses to mount without
// accessKeyId/accessKeySecret (src/driver/node.go). Both are returned, for the tests that
// also have to act as that user -- pre-creating a volume it must own, say.
//
// tenantIDs are the tenants the user is made admin of, that is the tenants the test
// provisions in. The first is also the tenant an access key is issued in, which is what the
// Quobyte API falls back to when neither the StorageClass nor the namespace mapping names
// one.
//
// The Secret is only built, not created -- the caller decides where and when. The Quobyte
// user and access key behind it, on the other hand, exist the moment this returns, so their
// removal is registered here, before the caller registers anything of its own: LIFO cleanup
// then revokes the key and deletes the user after whatever used them is gone.
func NewCredentials(t *testing.T, cfg Config, quobyteClient *quobyte.QuobyteClient, name string, tenantIDs ...string) (TestUser, *corev1.Secret) {
	t.Helper()

	if len(tenantIDs) == 0 || tenantIDs[0] == "" {
		t.Fatal("NewCredentials needs at least one tenant: the test's user is created as an admin of it")
	}

	user := CreateTestUser(t, cfg, quobyteClient, tenantIDs)

	if !cfg.EnableAccessKeyMounts {
		return user, NewSecret(name, cfg.Namespace, user.Name, user.Password)
	}

	// One key for both uses: this Secret is the provisioner secret and the mount secret
	// at once.
	credentials, err := CreateAccessKey(quobyteClient, tenantIDs[0], user.Name, GeneralAccessKey)
	if err != nil {
		t.Fatalf("creating a Quobyte access key for user %q: %v", user.Name, err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("quobyte access key %s", credentials.AccessKeyId), func() {
		if err := DeleteAccessKey(quobyteClient, user.Name, credentials.AccessKeyId); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	return user, NewAccessKeySecret(name, cfg.Namespace, credentials.AccessKeyId, credentials.SecretAccessKey)
}

// CreateTestUser creates the Quobyte user whose credentials a test hands to the driver,
// admin of every tenant in adminOfTenantIDs, and registers its removal.
//
// The API user from the environment is deliberately not used, for two reasons. It is shared
// by every test in a run, and granting it access to a tenant rewrites its tenant mappings:
// tests create and delete tenants of their own, so those mappings go stale and the next
// update is rejected with "tenant not found" (EnsureTenant prunes them for that reason).
// And an access key stands for a file system identity, so the user behind it needs a primary
// group, which the API user of an installation need not have -- Quobyte then refuses to
// issue the key at all. A user created for the run has neither problem, and that a plain
// tenant admin can drive the driver is worth showing anyway. This is the same setup the
// upstream suite makes for itself (kind-tests/e2e-upstream-tests/external_storage).
func CreateTestUser(t *testing.T, cfg Config, client *quobyte.QuobyteClient, adminOfTenantIDs []string) TestUser {
	t.Helper()

	suffix := time.Now().UnixNano()
	user := TestUser{
		Name:     fmt.Sprintf("e2e-user-%d", suffix),
		Password: fmt.Sprintf("e2e-pw-%d", suffix),
		// A group of its own where the user's access key has to stand for a file system
		// identity; where it does not, the group only has to exist.
		PrimaryGroup: "root",
	}
	if cfg.EnableAccessKeyMounts {
		user.PrimaryGroup = user.Name
	}

	// Provisioning through a shared volume means scheduling the delete files task on
	// cleanup, which an unprivileged user may not do.
	role := quobyte.UserRole_UNPRIVILEGED_USER
	if cfg.SharedVolumeOptions.EnableSharedVolume {
		role = quobyte.UserRole_FILESYSTEM_ADMIN
	}

	if err := CreateUser(client, user.Name, user.PrimaryGroup, user.Password, role, adminOfTenantIDs); err != nil {
		t.Fatalf("creating the quobyte user this test provisions through: %v", err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("quobyte user %s", user.Name), func() {
		if err := DeleteUser(client, user.Name); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	return user
}
