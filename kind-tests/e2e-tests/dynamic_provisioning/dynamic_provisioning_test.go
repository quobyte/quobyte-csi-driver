package dynamicprovisioning

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/e2e-tests/framework"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cleanupUnlessFailed registers fn to run during t.Cleanup, but skips it if the
// test has already failed -- leaving the Secret/StorageClass/PVC/Pod (and the
// CSI driver/client run_test deployed for this test) in the cluster so a failure
// can be debugged live with kubectl against the still-running KUBECONFIG cluster,
// instead of everything being torn down immediately.
func cleanupUnlessFailed(t *testing.T, description string, fn func()) {
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("test failed; leaving %s in place for debugging", description)
			return
		}
		fn()
	})
}

// TestDynamicProvisioningCreatesVolumeAndIsWritable exercises the basic
// dynamic-provisioning flow against a cluster set up by kind-tests/run_test.
// Every k8s resource involved -- Secret, StorageClass, PVC, Pod -- is
// generated in Go and uniquely named per run, so the test is fully
// self-contained: it doesn't depend on any pre-applied test-config manifest.
//
//   - create a Secret holding the Quobyte API credentials
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

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-dynprov-secret-%d", suffix)
	storageClassName := fmt.Sprintf("e2e-dynprov-sc-%d", suffix)
	pvcName := fmt.Sprintf("e2e-dynprov-pvc-%d", suffix)
	podName := fmt.Sprintf("e2e-dynprov-pod-%d", suffix)
	const mountPath = "/mnt/test"

	// Registered in dependency order (secret -> storage class -> pvc -> pod) so
	// t.Cleanup, which runs LIFO, tears down pod -> pvc -> storage class -> secret
	// on success: the pod is removed before its volume, and the secret the CSI
	// driver needs to delete the underlying Quobyte volume outlives the PVC
	// deletion that triggers it. On failure, cleanupUnlessFailed skips all of this
	// so the resources stay live for debugging (see run_test).
	secret := framework.NewSecret(secretName, cfg.Namespace, cfg.QuobyteAPIUser, cfg.QuobyteAPIPassword)
	_, err = clientset.CoreV1().Secrets(cfg.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	require.NoError(t, err, "creating secret")
	cleanupUnlessFailed(t, fmt.Sprintf("secret %s/%s", cfg.Namespace, secretName), func() {
		_ = clientset.CoreV1().Secrets(cfg.Namespace).Delete(context.Background(), secretName, metav1.DeleteOptions{})
	})

	storageClass := framework.NewStorageClass(storageClassName, cfg.CSIProvisionerName, cfg.QuobyteTenant, secretName, cfg.Namespace)
	_, err = clientset.StorageV1().StorageClasses().Create(ctx, storageClass, metav1.CreateOptions{})
	require.NoError(t, err, "creating storage class")
	cleanupUnlessFailed(t, fmt.Sprintf("storage class %s", storageClassName), func() {
		_ = clientset.StorageV1().StorageClasses().Delete(context.Background(), storageClassName, metav1.DeleteOptions{})
	})

	pvc := framework.NewPVC(pvcName, cfg.Namespace, storageClassName, "1Gi")
	_, err = clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	require.NoError(t, err, "creating pvc")
	cleanupUnlessFailed(t, fmt.Sprintf("pvc %s/%s", cfg.Namespace, pvcName), func() {
		_ = clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Delete(context.Background(), pvcName, metav1.DeleteOptions{})
	})

	boundPVC, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc to bind")

	pod := framework.NewPod(podName, cfg.Namespace, pvcName, mountPath)
	_, err = clientset.CoreV1().Pods(cfg.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err, "creating pod")
	cleanupUnlessFailed(t, fmt.Sprintf("pod %s/%s", cfg.Namespace, podName), func() {
		_ = clientset.CoreV1().Pods(cfg.Namespace).Delete(context.Background(), podName, metav1.DeleteOptions{})
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
