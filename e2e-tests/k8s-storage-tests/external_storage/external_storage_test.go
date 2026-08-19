package externalstorage

import (
	"context"
	"fmt"
	"testing"
	"time"

	quobyteApi "github.com/quobyte/api/v4/quobyte"
	"github.com/quobyte/quobyte-csi-driver/e2e-tests/framework"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestUpstreamExternalStorageSuite runs the upstream Kubernetes sig-storage
// "external storage" suite against the quobyte-csi-driver deployment
// e2e-tests/test_runner made for the environment being tested. It runs once per
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
//   - run: write the StorageClass to $ARTIFACTS_DIR and hand it to e2e-tests/e2e,
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
	// test_runner generates a fresh tenant name per run, but an "env" file may pin an
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
		// Registered right after the tenant, so LIFO cleanup runs it right before the
		// tenant is deleted -- a tenant that still holds volumes cannot be deleted. The
		// suite deletes its own PVCs, but the driver erases the volumes behind them and
		// erasing only schedules the erasure, so whatever is still listed here is
		// deleted outright. Inside this branch on purpose: emptying a tenant this run
		// did not create would be destructive.
		framework.CleanupUnlessFailed(t, fmt.Sprintf("the volumes of quobyte tenant %s", cfg.QuobyteTenant), func() {
			deleted, err := framework.DeleteVolumesInTenant(quobyteClient, tenantID)
			if deleted > 0 {
				t.Logf("deleted %d volume(s) still left in tenant %s", deleted, cfg.QuobyteTenant)
			}
			if err != nil {
				t.Logf("warning: %v", err)
			}
		})
	}

	primaryGroup := "root"
	if cfg.EnableAccessKeyMounts {
		primaryGroup = testUser
	}

	role := quobyteApi.UserRole_UNPRIVILEGED_USER
	// With a shared volume every PVC the suite creates becomes a subdirectory of this
	// one Quobyte volume instead of a volume of its own. The driver creates it on the
	// first provisioning request unless the environment asks the test to pre-create it.
	sharedVolume := cfg.SharedVolumeOptions
	if sharedVolume.EnableSharedVolume {
		// Should be able to start delete files task
		role = quobyteApi.UserRole_FILESYSTEM_ADMIN
	}

	// The suite provisions through a user of its own rather than the cluster admin,
	// which is also what proves a plain tenant admin is enough to drive the driver.
	require.NoError(t, framework.CreateUser(quobyteClient, testUser, primaryGroup, testUserPassword, role, []string{tenantID}),
		"creating quobyte user %q as admin of tenant %s", testUser, tenantID)
	framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte user %s", testUser), func() {
		if err := framework.DeleteUser(quobyteClient, testUser); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	// --- setup: Kubernetes side ----------------------------------------------
	// applySecret creates a Secret, dumps a copy of it next to the other artifacts and
	// registers its removal. The StorageClass pins its Secrets by name and namespace, so
	// they have to exist in the cluster for as long as the suite runs, even though the
	// StorageClass itself is only handed over as a file.
	applySecret := func(secret *corev1.Secret, purpose string) {
		_, err := clientset.CoreV1().Secrets(cfg.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		require.NoError(t, err, "creating %s secret", purpose)
		if path, err := framework.DumpSecretYAML(cfg.ArtifactsDir, fmt.Sprintf("%s-%s", t.Name(), purpose), secret); err != nil {
			t.Logf("warning: failed to dump %s secret artifact: %v", purpose, err)
		} else if path != "" {
			t.Logf("wrote %s secret artifact to %s", purpose, path)
		}
		framework.CleanupUnlessFailed(t, fmt.Sprintf("%s secret %s/%s", purpose, cfg.Namespace, secret.Name), func() {
			_ = clientset.CoreV1().Secrets(cfg.Namespace).Delete(context.Background(), secret.Name, metav1.DeleteOptions{})
		})
	}

	// newAccessKeySecret creates an access key of the given type for the test's user and
	// wraps it in a Secret. Registered for revocation before the Secret that carries it,
	// so LIFO cleanup revokes the key only after the suite (and its own volume cleanup)
	// is done with it.
	newAccessKeySecret := func(name string, keyType quobyteApi.AccessKeyType) *corev1.Secret {
		credentials, err := framework.CreateAccessKey(quobyteClient, tenantID, testUser, keyType)
		require.NoError(t, err, "creating a Quobyte %s for user %q", keyType, testUser)
		framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte access key %s", credentials.AccessKeyId), func() {
			if err := framework.DeleteAccessKey(quobyteClient, testUser, credentials.AccessKeyId); err != nil {
				t.Logf("warning: %v", err)
			}
		})
		return framework.NewAccessKeySecret(name, cfg.Namespace, credentials.AccessKeyId, credentials.SecretAccessKey)
	}

	// The driver puts a Secret to two different uses: management API credentials for
	// provisioning and expansion (src/driver/quobyte_api_client_factory.go), and file
	// system credentials for mounting (src/driver/node.go). Which credentials that is,
	// and whether one Secret covers both, is what the environment decides:
	//
	//   plain                     one Secret, the user's user/password
	//   access key mounts         one Secret, a general access key serving both uses
	//   + separate mount secret   two Secrets, a management access key for the API and a
	//                             data access key for mounting -- the setup
	//                             e2e-tests/test-configs/local_cluster_accesskeys_2
	//                             described on master
	//
	// The user created above is entitled to all of it, being an admin of the tenant the
	// suite provisions in.
	mountSecretName := ""
	switch {
	case cfg.EnableAccessKeyMounts && cfg.UseSeparateMountSecret:
		applySecret(newAccessKeySecret(secretName, framework.ManagementAccessKey), "api")

		mountSecretName = fmt.Sprintf("e2e-upstream-mount-secret-%d", suffix)
		applySecret(newAccessKeySecret(mountSecretName, framework.DataAccessKey), "mount")
	case cfg.EnableAccessKeyMounts:
		applySecret(newAccessKeySecret(secretName, framework.GeneralAccessKey), "api-and-mount")
	default:
		applySecret(framework.NewSecret(secretName, cfg.Namespace, testUser, testUserPassword), "api-and-mount")
	}

	if sharedVolume.EnableSharedVolume && sharedVolume.Name == "" {
		sharedVolume.Name = fmt.Sprintf("shared-%d", suffix)
	}
	if sharedVolume.PreCreateSharedVolume {
		sharedVolumeUUID, err := framework.CreateSharedVolume(quobyteClient, sharedVolume.Name, tenantID)
		require.NoError(t, err, "pre-creating shared volume %q", sharedVolume.Name)
		t.Logf("pre-created shared volume %s (%s)", sharedVolume.Name, sharedVolumeUUID)
		// The suite's own PVCs are subdirectories of this volume, so it can only go once
		// the suite has finished and removed them -- which it has by the time any cleanup
		// runs.
		framework.CleanupUnlessFailed(t, fmt.Sprintf("shared volume %s", sharedVolume.Name), func() {
			if err := framework.DeleteVolume(quobyteClient, sharedVolumeUUID); err != nil {
				t.Logf("warning: %v", err)
			}
		})
	}

	storageClass := framework.NewStorageClass(framework.StorageClassOptions{
		Name:            storageClassName,
		Provisioner:     cfg.CSIProvisionerName,
		Tenant:          cfg.QuobyteTenant,
		SecretName:      secretName,
		SecretNamespace: cfg.Namespace,
		// Empty unless the environment asked for a separate mount secret, in which case
		// node-publish points at it while provisioning/expansion keep the API secret.
		MountSecretName:      mountSecretName,
		MountSecretNamespace: cfg.Namespace,
		SharedVolumeName:     sharedVolume.Name,
	})
	storageClassFile, err := framework.DumpStorageClassYAML(cfg.ArtifactsDir, t.Name(), storageClass)
	require.NoError(t, err, "writing the StorageClass the upstream suite runs against")
	t.Logf("wrote storage class artifact to %s", storageClassFile)

	// --- run the upstream suite ----------------------------------------------
	require.NoError(t, framework.RunUpstreamE2E(ctx, cfg, framework.UpstreamE2EOptions{
		StorageClassFile: storageClassFile,
	}), "upstream Kubernetes external-storage suite")
}
