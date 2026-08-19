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

// TestDynamicProvisioningCreatesVolumeAndIsWritable exercises the basic
// dynamic-provisioning flow against a cluster set up by e2e-tests/test_runner.
// Every k8s resource involved -- Secret, StorageClass, PVC, Pod -- is
// generated in Go and uniquely named per run, so the test is fully
// self-contained: it doesn't depend on any pre-applied test-config manifest.
//
// It runs once per file in this directory's env/, i.e. once per driver setup
// test_runner deploys (see e2e-tests/test_runner), which is why the credentials it
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

	// With a shared volume the PVC becomes a subdirectory of this one Quobyte volume
	// instead of a volume of its own. The driver creates it on the first provisioning
	// request unless the environment asks the test to pre-create it.
	sharedVolume := cfg.SharedVolumeOptions
	if sharedVolume.EnableSharedVolume && sharedVolume.Name == "" {
		sharedVolume.Name = fmt.Sprintf("e2e-dynprov-shared-%d", suffix)
	}
	if sharedVolume.PreCreateSharedVolume {
		sharedVolumeUUID, err := framework.CreateSharedVolume(quobyteClient, sharedVolume.Name, tenantID)
		require.NoError(t, err, "pre-creating shared volume %q", sharedVolume.Name)
		t.Logf("pre-created shared volume %s (%s)", sharedVolume.Name, sharedVolumeUUID)
		// Registered before everything below, so LIFO cleanup removes the shared volume
		// last -- after the PVC, and with it the subdirectory inside this volume, is gone.
		framework.CleanupUnlessFailed(t, fmt.Sprintf("shared volume %s", sharedVolume.Name), func() {
			if err := framework.DeleteVolume(quobyteClient, sharedVolumeUUID); err != nil {
				t.Logf("warning: %v", err)
			}
		})
	}

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
	// so the resources stay live for debugging (see test_runner).
	//
	// Which credentials go into the Secret is decided by the environment the driver
	// was deployed with: with access key mounts enabled the node plugin requires
	// accessKeyId/accessKeySecret in the mount secret (src/driver/node.go), and the
	// same pair doubles as management API credentials for the provisioner
	// (src/driver/quobyte_api_client_factory.go). Either way they belong to a Quobyte
	// user created for this test alone, never the API user of the environment -- see
	// framework.CreateTestUser. Registered for cleanup before everything below, so LIFO
	// cleanup takes the credentials away only after the PVC is gone: the driver needs
	// them to delete the backing Quobyte volume.
	secret := framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID)
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

	storageClass := framework.NewStorageClass(framework.StorageClassOptions{
		Name:             storageClassName,
		Provisioner:      cfg.CSIProvisionerName,
		Tenant:           cfg.QuobyteTenant,
		SecretName:       secretName,
		SecretNamespace:  cfg.Namespace,
		SharedVolumeName: sharedVolume.Name,
	})
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

	// Both filled in below, once the PVC binds and its PV is inspected. The cleanups
	// registered here read them at teardown time, by when the PVC either bound (and
	// there is a PV and a Quobyte volume to deal with) or never did (and there is
	// nothing to do).
	var boundPVName string
	var quobyteVolumeUUID string

	// Registered before the PVC, so LIFO cleanup runs this right after the PVC and its
	// PV are gone. The driver erases the volume behind the PV, and erasing only
	// schedules the erasure, so the volume is deleted outright here to leave the tenant
	// empty. A volume the driver already removed is not an error.
	framework.CleanupUnlessFailed(t, "the Quobyte volume backing the test PVC", func() {
		if quobyteVolumeUUID == "" {
			return
		}
		if err := framework.DeleteVolume(quobyteClient, quobyteVolumeUUID); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	// What the tenant holds before this claim exists, so the count at the end is about this
	// claim rather than about everything the run has provisioned so far.
	volumesBefore, err := framework.ListVolumeNamesInTenant(quobyteClient, tenantID)
	require.NoError(t, err, "listing the volumes of tenant %s before provisioning", cfg.QuobyteTenant)

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

	tenantUUID, volumeUUID, subDir, err := splitVolumeHandle(pv.Spec.CSI.VolumeHandle)
	require.NoError(t, err, "parsing PV volume handle")
	if sharedVolume.Name != "" {
		// Provisioning through a shared volume means the PVC is a subdirectory of it,
		// which the handle spells out as a third part.
		require.NotEmpty(t, subDir,
			"PV %s was provisioned through shared volume %q but its handle %q names no subdirectory",
			pv.Name, sharedVolume.Name, pv.Spec.CSI.VolumeHandle)
	} else {
		// Hands the volume to the cleanup registered before the PVC, above. Not done
		// for a shared volume: there the volume is not the test's to delete -- it
		// outlives the PVC and belongs either to the driver or to the pre-create
		// cleanup registered above.
		quobyteVolumeUUID = volumeUUID
	}

	gotUUID, err := quobyteClient.GetVolumeUUID(volumeUUID, tenantUUID)
	require.NoError(t, err, "volume referenced by PV %s not found in Quobyte", pv.Name)
	require.Equal(t, volumeUUID, gotUUID, "Quobyte API returned a different UUID than the PV volume handle")

	// One claim, one volume. A second one means the driver provisioned twice for the same
	// claim -- which is what a controller deployed with more than one replica does when
	// leader election is not working (see env/ha_controller). Only for a volume per claim:
	// through a shared volume the claim adds a subdirectory, and how many volumes the tenant
	// gains depends on whether the shared volume was pre-created, which shared_volume/
	// covers instead.
	if sharedVolume.Name == "" {
		volumesAfter, err := framework.ListVolumeNamesInTenant(quobyteClient, tenantID)
		require.NoError(t, err, "listing the volumes of tenant %s after provisioning", cfg.QuobyteTenant)
		require.Len(t, volumesAfter, len(volumesBefore)+1,
			"one claim should have added exactly one volume to tenant %s: it held %v before and holds %v now",
			cfg.QuobyteTenant, volumesBefore, volumesAfter)
	}
}

// splitVolumeHandle parses a CSI VolumeHandle of the form "tenantUUID|volumeUUID", or
// "tenantUUID|volumeUUID|subDir" when the volume was provisioned as a subdirectory of a
// shared volume (see VOLUME_HANDLE_PART_SEPARATOR in src/driver/controller.go). subDir is
// empty for the two-part form.
func splitVolumeHandle(handle string) (tenantUUID, volumeUUID, subDir string, err error) {
	parts := strings.Split(handle, "|")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("unexpected volume handle format %q", handle)
	}
	if len(parts) > 2 {
		subDir = parts[2]
	}
	return parts[0], parts[1], subDir, nil
}
