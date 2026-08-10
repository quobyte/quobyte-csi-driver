package externalstorage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestUpstreamExternalStorageSuite runs the upstream Kubernetes sig-storage
// "external storage" suite against the quobyte-csi-driver deployment
// kind-tests/run_test made for the environment being tested. It runs once per
// file in this directory's env/, so the same suite is exercised against every
// driver setup listed there.
//
// Unlike the sanity tests, the suite itself is not ours -- it comes from the
// released kubernetes-test tarball and only knows how to create PVCs from a
// StorageClass. So this test brackets it:
//
//   - setup: create the tenant and a dedicated Quobyte user for this run through
//     the Quobyte Go API, then the k8s Secret holding that user's credentials
//     (an access key of that user, where the environment mounts with access keys)
//   - run: write the StorageClass to $ARTIFACTS_DIR and hand it to kind-tests/e2e,
//     which passes it to e2e.test as "StorageClass: FromFile"
//   - cleanup: delete the Secret, the user and the tenant again (the suite cleans
//     up its own namespaces, PVCs and StorageClass copies)
//
// On failure nothing is cleaned up, so the cluster and the Quobyte-side setup can
// be inspected -- see framework.CleanupUnlessFailed.
func TestUpstreamExternalStorageSuite(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	require.NotEmpty(t, cfg.ArtifactsDir,
		"ARTIFACTS_DIR must be set: the upstream suite consumes the StorageClass as a file")

	clientset, _, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	suffix := time.Now().UnixNano()
	testUser := fmt.Sprintf("e2e-upstream-%d", suffix)
	testUserPassword := fmt.Sprintf("e2e-upstream-pw-%d", suffix)
	secretName := fmt.Sprintf("e2e-upstream-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-upstream-sc-%d", suffix)
	}

	// --- setup: Quobyte side -------------------------------------------------
	// run_test generates a fresh tenant name per run, but an "env" file may pin an
	// existing one via QUOBYTE_TENANT. Only a tenant this run brought into
	// existence may be deleted again afterwards.
	_, err = quobyteClient.ResolveTenantNameToUUID(cfg.QuobyteTenant)
	tenantPreExisted := err == nil

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	if !tenantPreExisted {
		framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte tenant %s", cfg.QuobyteTenant), func() {
			if err := framework.DeleteTenant(quobyteClient, cfg.QuobyteTenant); err != nil {
				t.Logf("warning: %v", err)
			}
		})
	}

	primaryGroup := "root"
	if cfg.EnableAccessKeyMounts {
		primaryGroup = testUser
	}

	// The suite provisions through a user of its own rather than the cluster admin,
	// which is also what proves a plain tenant admin is enough to drive the driver.
	require.NoError(t, framework.CreateUser(quobyteClient, testUser, primaryGroup, testUserPassword, []string{tenantID}),
		"creating quobyte user %q as admin of tenant %s", testUser, tenantID)
	framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte user %s", testUser), func() {
		if err := framework.DeleteUser(quobyteClient, testUser); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	// --- setup: Kubernetes side ----------------------------------------------
	// The StorageClass pins this Secret by name and namespace, so it has to exist in
	// the cluster for as long as the suite runs, even though the StorageClass itself
	// is only handed over as a file.
	//
	// Which of the user's credentials go into it depends on the environment the driver
	// was deployed with: with access key mounts enabled the node plugin requires
	// accessKeyId/accessKeySecret in the mount secret (src/driver/node.go), and the same
	// pair doubles as management API credentials for the provisioner
	// (src/driver/quobyte_api_client_factory.go) -- which the user above is entitled to
	// use, being an admin of the tenant the suite provisions in.
	var secret *corev1.Secret
	if cfg.EnableAccessKeyMounts {
		credentials, err := framework.CreateAccessKey(quobyteClient, tenantID, testUser)
		require.NoError(t, err, "creating a Quobyte access key for user %q", testUser)
		// Registered before the Secret, so LIFO cleanup revokes the key only after the
		// suite (and its own volume cleanup) is done with it.
		framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte access key %s", credentials.AccessKeyId), func() {
			if err := framework.DeleteAccessKey(quobyteClient, testUser, credentials.AccessKeyId); err != nil {
				t.Logf("warning: %v", err)
			}
		})
		secret = framework.NewAccessKeySecret(secretName, cfg.Namespace, credentials.AccessKeyId, credentials.SecretAccessKey)
	} else {
		secret = framework.NewSecret(secretName, cfg.Namespace, testUser, testUserPassword)
	}
	_, err = clientset.CoreV1().Secrets(cfg.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	require.NoError(t, err, "creating secret")
	if path, err := framework.DumpSecretYAML(cfg.ArtifactsDir, t.Name(), secret); err != nil {
		t.Logf("warning: failed to dump secret artifact: %v", err)
	} else if path != "" {
		t.Logf("wrote secret artifact to %s", path)
	}
	framework.CleanupUnlessFailed(t, fmt.Sprintf("secret %s/%s", cfg.Namespace, secretName), func() {
		_ = clientset.CoreV1().Secrets(cfg.Namespace).Delete(context.Background(), secretName, metav1.DeleteOptions{})
	})

	storageClass := framework.NewStorageClass(storageClassName, cfg.CSIProvisionerName, cfg.QuobyteTenant, secretName, cfg.Namespace)
	storageClassFile, err := framework.DumpStorageClassYAML(cfg.ArtifactsDir, t.Name(), storageClass)
	require.NoError(t, err, "writing the StorageClass the upstream suite runs against")
	t.Logf("wrote storage class artifact to %s", storageClassFile)

	// --- run the upstream suite ----------------------------------------------
	require.NoError(t, framework.RunUpstreamE2E(ctx, cfg, framework.UpstreamE2EOptions{
		StorageClassFile: storageClassFile,
	}), "upstream Kubernetes external-storage suite")
}
