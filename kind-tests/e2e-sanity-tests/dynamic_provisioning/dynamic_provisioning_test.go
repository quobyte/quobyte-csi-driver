package dynamicprovisioning

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestDynamicProvisioningCreatesVolumeAndIsWritable exercises the basic
// dynamic-provisioning flow against a cluster set up by kind-tests/run_test.
// Every k8s resource involved -- Secret, StorageClass, PVC, Pod -- is
// generated in Go and uniquely named per run, so the test is fully
// self-contained: it doesn't depend on any pre-applied test-config manifest.
//
// It runs once per file in this directory's env/, i.e. once per driver setup
// run_test deploys (see kind-tests/run_test), which is why the credentials it
// puts into its Secret depend on cfg.EnableAccessKeyMounts.
//
//   - create a Secret holding the Quobyte credentials
//   - create a StorageClass referencing that secret
//   - create a PVC against that StorageClass and wait for it to bind
//   - create a pod mounting that PVC and wait for it to run
//   - write and read back a file through the mount, proving the CSI mount is
//     genuinely read/write
//   - resolve the bound PV's VolumeHandle (tenantUUID|volumeUUID) and confirm
//     the volume actually exists in Quobyte via the same API client the
//     driver itself uses
func TestDynamicProvisioningCreatesVolumeAndIsWritable(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-dynprov-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-dynprov-sc-%d", suffix)
	}
	pvcName := fmt.Sprintf("e2e-dynprov-pvc-%d", suffix)
	podName := fmt.Sprintf("e2e-dynprov-pod-%d", suffix)
	const mountPath = "/mnt/test"

	// Shared prefix for the artifact copies of the Secret/StorageClass dumped
	// below, so what this run actually applied is still inspectable in
	// $ARTIFACTS_DIR after the test's own cleanup deleted the live objects.
	artifactPrefix := fmt.Sprintf("%s-%d", t.Name(), suffix)

	// Everything below is registered for cleanup in dependency order (secret ->
	// storage class -> pvc -> pod) so t.Cleanup, which runs LIFO, tears down
	// pod -> pvc -> storage class -> secret on success: the pod is removed before
	// its volume, and the secret the CSI driver needs to delete the underlying
	// Quobyte volume outlives the PVC deletion that triggers it. Deleting a k8s
	// object only marks it for deletion, so each step also waits for its resource
	// to actually be gone -- otherwise the steps overlap and that order is only
	// nominal. A wait that times out is reported as a warning and the remaining
	// steps still run. On failure, framework.CleanupUnlessFailed skips all of this
	// so the resources stay live for debugging (see run_test).
	//
	// Which credentials go into the Secret is decided by the environment the driver
	// was deployed with: with access key mounts enabled the node plugin requires
	// accessKeyId/accessKeySecret in the mount secret (src/driver/node.go), and the
	// same pair doubles as management API credentials for the provisioner
	// (src/driver/quobyte_api_client_factory.go).
	var secret *corev1.Secret
	if cfg.EnableAccessKeyMounts {
		credentials, err := framework.CreateAccessKey(quobyteClient, tenantID, cfg.QuobyteAPIUser)
		require.NoError(t, err, "creating a Quobyte access key for user %q", cfg.QuobyteAPIUser)
		// Registered before everything below, so LIFO cleanup revokes the key only
		// after the PVC is gone -- the driver needs these credentials to delete the
		// backing Quobyte volume.
		framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte access key %s", credentials.AccessKeyId), func() {
			if err := framework.DeleteAccessKey(quobyteClient, cfg.QuobyteAPIUser, credentials.AccessKeyId); err != nil {
				t.Logf("warning: %v", err)
			}
		})
		secret = framework.NewAccessKeySecret(secretName, cfg.Namespace, credentials.AccessKeyId, credentials.SecretAccessKey)
	} else {
		secret = framework.NewSecret(secretName, cfg.Namespace, cfg.QuobyteAPIUser, cfg.QuobyteAPIPassword)
	}
	_, err = clientset.CoreV1().Secrets(cfg.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	require.NoError(t, err, "creating secret")
	if path, err := framework.DumpSecretYAML(cfg.ArtifactsDir, artifactPrefix, secret); err != nil {
		t.Logf("warning: failed to dump secret artifact: %v", err)
	} else if path != "" {
		t.Logf("wrote secret artifact to %s", path)
	}
	framework.CleanupUnlessFailed(t, fmt.Sprintf("secret %s/%s", cfg.Namespace, secretName), func() {
		cleanupCtx := context.Background()
		_ = clientset.CoreV1().Secrets(cfg.Namespace).Delete(cleanupCtx, secretName, metav1.DeleteOptions{})
		if err := framework.WaitForSecretDeleted(cleanupCtx, clientset, cfg.Namespace, secretName, time.Minute); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	storageClass := framework.NewStorageClass(storageClassName, cfg.CSIProvisionerName, cfg.QuobyteTenant, secretName, cfg.Namespace)
	_, err = clientset.StorageV1().StorageClasses().Create(ctx, storageClass, metav1.CreateOptions{})
	require.NoError(t, err, "creating storage class")
	if path, err := framework.DumpStorageClassYAML(cfg.ArtifactsDir, artifactPrefix, storageClass); err != nil {
		t.Logf("warning: failed to dump storage class artifact: %v", err)
	} else if path != "" {
		t.Logf("wrote storage class artifact to %s", path)
	}
	framework.CleanupUnlessFailed(t, fmt.Sprintf("storage class %s", storageClassName), func() {
		cleanupCtx := context.Background()
		_ = clientset.StorageV1().StorageClasses().Delete(cleanupCtx, storageClassName, metav1.DeleteOptions{})
		if err := framework.WaitForStorageClassDeleted(cleanupCtx, clientset, storageClassName, time.Minute); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	// Filled in once the PVC binds, below. The cleanup registered here reads it at
	// teardown time, by when the PVC either bound (and there is a PV whose deletion
	// has to complete) or never did (and there is nothing to wait for).
	var boundPVName string

	pvc := framework.NewPVC(pvcName, cfg.Namespace, storageClassName, "1Gi")
	_, err = clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	require.NoError(t, err, "creating pvc")
	framework.CleanupUnlessFailed(t, fmt.Sprintf("pvc %s/%s", cfg.Namespace, pvcName), func() {
		cleanupCtx := context.Background()
		_ = clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Delete(cleanupCtx, pvcName, metav1.DeleteOptions{})
		if err := framework.WaitForPVCDeleted(cleanupCtx, clientset, cfg.Namespace, pvcName, 2*time.Minute); err != nil {
			t.Logf("warning: %v", err)
		}
		// The PV going away is what shows the driver finished deleting the backing
		// Quobyte volume. It has to happen before the remaining cleanup steps take
		// away the credentials the driver needs for exactly that.
		if boundPVName != "" {
			if err := framework.WaitForPVDeleted(cleanupCtx, clientset, boundPVName, 3*time.Minute); err != nil {
				t.Logf("warning: %v", err)
			}
		}
	})

	boundPVC, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc to bind")
	boundPVName = boundPVC.Spec.VolumeName

	pod := framework.NewPod(podName, cfg.Namespace, pvcName, mountPath)
	_, err = clientset.CoreV1().Pods(cfg.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err, "creating pod")
	framework.CleanupUnlessFailed(t, fmt.Sprintf("pod %s/%s", cfg.Namespace, podName), func() {
		cleanupCtx := context.Background()
		_ = clientset.CoreV1().Pods(cfg.Namespace).Delete(cleanupCtx, podName, metav1.DeleteOptions{})
		// The pod has to be gone, not just terminating, before the next cleanup
		// step tears out the volume it still has mounted.
		if err := framework.WaitForPodDeleted(cleanupCtx, clientset, cfg.Namespace, podName, 2*time.Minute); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, podName, 2*time.Minute)
	require.NoError(t, err, "waiting for pod to run")

	testFile := mountPath + "/e2e-test.txt"
	wantContent := fmt.Sprintf("e2e-%d", suffix)

	_, stderr, err := framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, podName, "test-container",
		[]string{"sh", "-c", fmt.Sprintf("echo -n %s > %s", wantContent, testFile)})
	require.NoErrorf(t, err, "writing file into mounted volume: stderr=%s", stderr)

	stdout, stderr, err := framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, podName, "test-container",
		[]string{"cat", testFile})
	require.NoErrorf(t, err, "reading file back from mounted volume: stderr=%s", stderr)
	require.Equal(t, wantContent, strings.TrimSpace(stdout), "file content read back from the Quobyte mount did not match what was written")

	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, boundPVC.Spec.VolumeName, metav1.GetOptions{})
	require.NoError(t, err, "fetching bound PV")
	require.NotNil(t, pv.Spec.CSI, "bound PV has no CSI volume source")

	tenantUUID, volumeUUID, err := splitVolumeHandle(pv.Spec.CSI.VolumeHandle)
	require.NoError(t, err, "parsing PV volume handle")

	gotUUID, err := quobyteClient.GetVolumeUUID(volumeUUID, tenantUUID)
	require.NoError(t, err, "volume referenced by PV %s not found in Quobyte", pv.Name)
	require.Equal(t, volumeUUID, gotUUID, "Quobyte API returned a different UUID than the PV volume handle")
}

// splitVolumeHandle parses a CSI VolumeHandle of the form "tenantUUID|volumeUUID"
// (see VOLUME_HANDLE_PART_SEPARATOR in src/driver/controller.go) into its parts.
func splitVolumeHandle(handle string) (tenantUUID, volumeUUID string, err error) {
	parts := strings.Split(handle, "|")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected volume handle format %q", handle)
	}
	return parts[0], parts[1], nil
}
