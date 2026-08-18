package expansion

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	mountPath = "/mnt/test"
	// One gibibyte to start with, two after the resize -- small enough that the quota is the
	// only thing that has to move, large enough that the two are unmistakable in a message.
	initialSize = "1Gi"
	grownSize   = "2Gi"
)

// TestVolumeExpansionMovesTheQuobyteQuota covers ControllerExpandVolume, which until now was
// only exercised indirectly by the upstream sig-storage suite -- and that suite checks the
// Kubernetes objects, not the storage system. A driver that reported success without ever
// calling SetVolumeQuota would pass it.
//
// The two env files of this package are the two branches of expandVolume
// (src/driver/controller.go):
//
//	volume per claim   the handle has two parts, the new size becomes the volume's quota
//	shared volume      the handle has three, and expansion is a deliberate no-op -- the
//	                   claim is a subdirectory with no quota of its own, and returning
//	                   success is what lets a customer resize the PVC instead of having to
//	                   destroy and recreate it
//
// Both assert the same thing about Kubernetes (the claim reports the new size) and the
// opposite thing about Quobyte, which is the point: success here does not mean the same
// thing on both sides.
func TestVolumeExpansionMovesTheQuobyteQuota(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-expand-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-expand-sc-%d", suffix)
	}
	pvcName := fmt.Sprintf("e2e-expand-pvc-%d", suffix)
	podName := fmt.Sprintf("e2e-expand-pod-%d", suffix)

	sharedVolume := cfg.SharedVolumeOptions
	if sharedVolume.EnableSharedVolume && sharedVolume.Name == "" {
		sharedVolume.Name = fmt.Sprintf("e2e-expand-shared-%d", suffix)
	}
	if sharedVolume.PreCreateSharedVolume {
		sharedVolumeUUID, err := framework.CreateSharedVolume(quobyteClient, sharedVolume.Name, tenantID)
		require.NoError(t, err, "pre-creating shared volume %q", sharedVolume.Name)
		framework.CleanupUnlessFailed(t, fmt.Sprintf("shared volume %s", sharedVolume.Name), func() {
			if err := framework.DeleteVolume(quobyteClient, sharedVolumeUUID); err != nil {
				t.Logf("warning: %v", err)
			}
		})
	}

	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID))

	// createQuota is on by default in NewStorageClass, and has to be: without it the driver
	// never sets a quota at provisioning time and there would be nothing for the expansion
	// to move. AllowVolumeExpansion is set there too.
	framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:             storageClassName,
		Provisioner:      cfg.CSIProvisionerName,
		Tenant:           cfg.QuobyteTenant,
		SecretName:       secretName,
		SecretNamespace:  cfg.Namespace,
		SharedVolumeName: sharedVolume.Name,
	}))

	framework.ApplyPVC(t, ctx, clientset, framework.NewPVC(pvcName, cfg.Namespace, storageClassName, initialSize))
	boundPVC, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc to bind")

	framework.ApplyPod(t, ctx, clientset, framework.NewPod(podName, cfg.Namespace, pvcName, mountPath))
	_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, podName, 2*time.Minute)
	require.NoError(t, err, "waiting for pod to run")

	// Written before the resize and read after it: growing a volume must not disturb what is
	// already in it, which is the part a customer would notice first.
	const canaryFile = "canary.txt"
	canaryContent := fmt.Sprintf("expand-%d", suffix)
	require.NoError(t, framework.WriteFileInPod(ctx, restConfig, clientset, cfg.Namespace, podName,
		mountPath+"/"+canaryFile, canaryContent), "writing into the mounted volume before the resize")

	_, volumeUUID, subDir, err := splitVolumeHandle(volumeHandleOf(t, ctx, clientset, boundPVC.Spec.VolumeName))
	require.NoError(t, err, "parsing the bound PV's volume handle")
	isSharedVolumeClaim := subDir != ""
	require.Equal(t, sharedVolume.EnableSharedVolume, isSharedVolumeClaim,
		"USE_SHARED_VOLUME says %t but the PV handle says otherwise, so this run is not testing the "+
			"branch its env file selected", sharedVolume.EnableSharedVolume)

	quotaBefore, hadQuota, err := framework.GetVolumeQuota(quobyteClient, volumeUUID)
	require.NoError(t, err, "reading the quota of volume %s before the resize", volumeUUID)
	initial := mustParse(t, initialSize)
	if !isSharedVolumeClaim {
		require.True(t, hadQuota, "the claim was provisioned with createQuota but the volume has no quota")
		require.Equal(t, initial.Value(), quotaBefore,
			"the volume's quota does not match the size the claim asked for")
	}

	// --- grow the claim -------------------------------------------------------
	grown := mustParse(t, grownSize)
	patched, err := clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Get(ctx, pvcName, metav1.GetOptions{})
	require.NoError(t, err, "fetching pvc %s to resize it", pvcName)
	patched.Spec.Resources.Requests[corev1.ResourceStorage] = grown
	_, err = clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Update(ctx, patched, metav1.UpdateOptions{})
	require.NoError(t, err, "asking for pvc %s to grow to %s", pvcName, grownSize)

	_, err = framework.WaitForPVCCapacity(ctx, clientset, cfg.Namespace, pvcName, grown, 3*time.Minute)
	require.NoError(t, err, "the claim never reported the size it was resized to")

	// --- what that meant in Quobyte -------------------------------------------
	quotaAfter, hasQuota, err := framework.GetVolumeQuota(quobyteClient, volumeUUID)
	require.NoError(t, err, "reading the quota of volume %s after the resize", volumeUUID)

	if isSharedVolumeClaim {
		// The claim is a subdirectory; the quota, if any, belongs to the shared volume and
		// to whoever set it, and the driver must not have touched it on the claim's behalf.
		require.Equal(t, hadQuota, hasQuota,
			"expanding a claim inside a shared volume changed whether the shared volume has a quota")
		require.Equal(t, quotaBefore, quotaAfter,
			"expanding a claim inside a shared volume moved the shared volume's quota from %d to %d, "+
				"which would resize every other claim in it", quotaBefore, quotaAfter)
	} else {
		require.True(t, hasQuota, "the volume lost its quota during the resize")
		require.Equal(t, grown.Value(), quotaAfter,
			"the claim reports %s but the volume's Quobyte quota is %d bytes, so the resize never "+
				"reached the storage system", grownSize, quotaAfter)
	}

	// --- and the data is still there ------------------------------------------
	got, err := framework.ReadFileInPod(ctx, restConfig, clientset, cfg.Namespace, podName, mountPath+"/"+canaryFile)
	require.NoError(t, err, "reading the canary file back after the resize")
	require.Equal(t, canaryContent, strings.TrimSpace(got), "the resize disturbed what was in the volume")
}

// volumeHandleOf returns the CSI volume handle of a PV -- the driver's own name for what it
// provisioned, and the only place the shape of the claim (a volume of its own, or a
// subdirectory of a shared one) is stated.
func volumeHandleOf(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, pvName string) string {
	t.Helper()

	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	require.NoError(t, err, "fetching PV %s", pvName)
	require.NotNil(t, pv.Spec.CSI, "PV %s has no CSI volume source", pvName)

	return pv.Spec.CSI.VolumeHandle
}

// splitVolumeHandle parses "tenantUUID|volumeUUID", or "tenantUUID|volumeUUID|subDir" when
// the claim was provisioned as a subdirectory of a shared volume (see
// VOLUME_HANDLE_PART_SEPARATOR in src/driver/controller.go). subDir is empty for the
// two-part form, which is what tells the two branches of expandVolume apart.
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

func mustParse(t *testing.T, size string) resourceapi.Quantity {
	t.Helper()

	quantity, err := resourceapi.ParseQuantity(size)
	require.NoError(t, err, "parsing size %q", size)

	return quantity
}
