package accesskeynegative

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
)

// TestAccessKeyCannotProvisionIntoAnotherTenant is the isolation case the rest of this
// package does not cover. The two cases in access_key_negative_test.go are about credentials
// that are wrong or of the wrong kind; this one is about credentials that are perfectly
// valid -- and belong to somebody else.
//
// An access key is issued inside one tenant. A StorageClass names the tenant to provision
// into (quobyteTenant, see CreateVolume in src/driver/controller.go) and the driver passes
// both along without comparing them, because it is the Quobyte API that decides what a key
// may reach. So this test asserts what the installation promises rather than what the driver
// computes: a key issued in tenant A must not be able to create a volume in tenant B.
//
// A pass here would mean tenant separation is not enforced at all -- the one failure in this
// suite that would be a security finding rather than a bug.
func TestAccessKeyCannotProvisionIntoAnotherTenant(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	require.True(t, cfg.EnableAccessKeyMounts,
		"this test needs access key credentials; check this directory's env file")

	clientset, _, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	// The tenant the StorageClass names, and the one the volume must never appear in. This
	// is the run's own tenant, so anything in it came from this run.
	targetTenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	// The tenant the key belongs to, created for this test and given back afterwards.
	otherTenantName := fmt.Sprintf("%s-other-%d", cfg.QuobyteTenant, time.Now().UnixNano())
	otherTenantID, err := framework.EnsureTenant(quobyteClient, otherTenantName, cfg.QuobyteAPIUser)
	require.NoError(t, err, "creating tenant %q for the key under test", otherTenantName)
	framework.CleanupUnlessFailed(t, fmt.Sprintf("tenant %s", otherTenantName), func() {
		if _, err := framework.DeleteVolumesInTenant(quobyteClient, otherTenantID); err != nil {
			t.Logf("warning: %v", err)
		}
		if err := framework.DeleteTenant(quobyteClient, otherTenantName); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	// Admin of the other tenant and of nothing else: this user has every right it needs --
	// in its own tenant.
	keyOwner := framework.CreateTestUser(t, cfg, quobyteClient, []string{otherTenantID})
	credentials, err := framework.CreateAccessKey(quobyteClient, otherTenantID, keyOwner.Name, framework.GeneralAccessKey)
	require.NoError(t, err, "issuing an access key for %q in tenant %q", keyOwner.Name, otherTenantName)
	framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte access key %s", credentials.AccessKeyId), func() {
		if err := framework.DeleteAccessKey(quobyteClient, keyOwner.Name, credentials.AccessKeyId); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	// What the target tenant holds before the attempt, so "no volume was created" can be
	// stated about this attempt rather than about the tenant's whole history.
	before, err := framework.ListVolumeNamesInTenant(quobyteClient, targetTenantID)
	require.NoError(t, err, "listing the volumes of tenant %q before the attempt", cfg.QuobyteTenant)

	names := newNames(t, "crosstenant")
	framework.ApplySecret(t, ctx, clientset,
		framework.NewAccessKeySecret(names.secret, cfg.Namespace, credentials.AccessKeyId, credentials.SecretAccessKey))
	// Names the target tenant, which this key has no business in.
	applyStorageClassAndClaim(t, ctx, clientset, cfg, names)

	require.NoError(t, framework.EnsurePVCStaysUnbound(ctx, clientset, cfg.Namespace, names.pvc, stayFailedFor),
		"a volume was provisioned into tenant %q with an access key issued in tenant %q, so tenant "+
			"separation is not enforced", cfg.QuobyteTenant, otherTenantName)

	message, err := framework.WaitForEventMatching(ctx, clientset, cfg.Namespace, names.pvc, "failed to provision volume", time.Minute)
	require.NoError(t, err, "the PVC never bound, but no provisioning failure was reported on it")
	t.Logf("provisioning into another tenant was refused with: %s", message)

	after, err := framework.ListVolumeNamesInTenant(quobyteClient, targetTenantID)
	require.NoError(t, err, "listing the volumes of tenant %q after the attempt", cfg.QuobyteTenant)
	require.ElementsMatch(t, before, after,
		"the claim never bound, but tenant %q gained a volume during the attempt", cfg.QuobyteTenant)
}
