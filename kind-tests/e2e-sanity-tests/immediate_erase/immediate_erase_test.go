package immediateerase

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/quobyte/api/v4/quobyte"
	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	mountPath = "/mnt/test"
	// How long the volume is given to disappear after its PV did. Erasing is a task in
	// Quobyte, not a synchronous call, so this is not instant even when it is scheduled
	// immediately -- but for a volume holding one small file it is a matter of seconds,
	// and anything approaching this timeout means the erase was queued for later instead.
	eraseTimeout = 3 * time.Minute
)

// TestImmediateEraseRemovesVolumeWithThePV covers the quobyte.immediateErase flag, which no
// other environment in this directory turns on.
//
// DeleteVolume passes d.ImmediateErase straight to EraseVolumeByResolvingNamesToUUID
// (src/driver/controller.go). Erasing a volume normally only schedules the erasure, which is
// why framework.DeleteVolume exists at all: a volume the driver "deleted" can still be
// listed in its tenant long afterwards, so no other test here can say anything about when it
// really goes. With immediateErase the erase task is scheduled at once, and that is what this
// test asserts -- the volume is gone from its tenant shortly after the PV is.
//
// A regression makes this test hang until eraseTimeout rather than fail outright, which is
// the honest outcome: "the volume did not go away in three minutes" is exactly the claim.
func TestImmediateEraseRemovesVolumeWithThePV(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-erase-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-erase-sc-%d", suffix)
	}
	pvcName := fmt.Sprintf("e2e-erase-pvc-%d", suffix)
	podName := fmt.Sprintf("e2e-erase-pod-%d", suffix)

	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID))

	framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:            storageClassName,
		Provisioner:     cfg.CSIProvisionerName,
		Tenant:          cfg.QuobyteTenant,
		SecretName:      secretName,
		SecretNamespace: cfg.Namespace,
	}))

	framework.ApplyPVC(t, ctx, clientset, framework.NewPVC(pvcName, cfg.Namespace, storageClassName, "1Gi"))
	boundPVC, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc to bind")

	// The volume the driver created is named after the PV (CreateVolume uses req.Name), so
	// this is what to look for in the tenant afterwards.
	volumeName := boundPVC.Spec.VolumeName
	require.NotEmpty(t, volumeName, "the bound claim names no PV")

	framework.ApplyPod(t, ctx, clientset, framework.NewPod(podName, cfg.Namespace, pvcName, mountPath))
	_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, podName, 2*time.Minute)
	require.NoError(t, err, "waiting for pod to run")

	// Erasing an empty volume would prove less: what the flag brings forward is the removal
	// of the data, so there has to be some.
	require.NoError(t, framework.WriteFileInPod(ctx, restConfig, clientset, cfg.Namespace, podName,
		mountPath+"/erase-me.txt", fmt.Sprintf("erase-%d", suffix)), "writing into the mounted volume")

	require.Contains(t, volumeNamesIn(t, quobyteClient, tenantID), volumeName,
		"the volume backing the bound claim is not in tenant %s, so nothing below would mean anything",
		cfg.QuobyteTenant)

	// --- deleting the claim takes the volume with it, now ---------------------
	require.NoError(t, clientset.CoreV1().Pods(cfg.Namespace).Delete(ctx, podName, metav1.DeleteOptions{}),
		"deleting pod %s", podName)
	require.NoError(t, framework.WaitForPodDeleted(ctx, clientset, cfg.Namespace, podName, 2*time.Minute))
	require.NoError(t, clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Delete(ctx, pvcName, metav1.DeleteOptions{}),
		"deleting pvc %s", pvcName)
	require.NoError(t, framework.WaitForPVCDeleted(ctx, clientset, cfg.Namespace, pvcName, 2*time.Minute))
	require.NoError(t, framework.WaitForPVDeleted(ctx, clientset, volumeName, 3*time.Minute),
		"the PV was never removed, so the driver's DeleteVolume did not succeed")

	deadline := time.Now().Add(eraseTimeout)
	for {
		names := volumeNamesIn(t, quobyteClient, tenantID)
		if !slices.Contains(names, volumeName) {
			t.Logf("volume %s was erased and is gone from tenant %s", volumeName, cfg.QuobyteTenant)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("volume %s is still in tenant %s %s after its PV was deleted, so the erase was "+
				"queued for later rather than scheduled immediately; the tenant holds %v",
				volumeName, cfg.QuobyteTenant, eraseTimeout, names)
		}
		time.Sleep(5 * time.Second)
	}
}

func volumeNamesIn(t *testing.T, client *quobyte.QuobyteClient, tenantID string) []string {
	t.Helper()

	names, err := framework.ListVolumeNamesInTenant(client, tenantID)
	require.NoError(t, err, "listing the volumes of tenant %s", tenantID)

	return names
}
