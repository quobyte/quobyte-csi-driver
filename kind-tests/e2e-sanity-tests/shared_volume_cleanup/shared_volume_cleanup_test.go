package sharedvolumecleanup

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// Where the claim under test is mounted, and where the shared volume's root is mounted
	// alongside it -- the second one is what makes the driver's cleanup observable at all.
	claimMountPath = "/mnt/claim"
	rootMountPath  = "/mnt/shared-volume-root"
)

// TestSharedVolumeCleanupRenamesSubdirectory covers the cleanup half of the "Shared volume"
// entry in kind-tests/e2e-sanity-tests/README.md, which shared_volume/ deliberately leaves
// out.
//
// Deleting a shared volume PVC takes one of two paths (DeleteVolume, src/driver/controller.go):
//
//	useDeleteFilesTask=true   a DELETE_FILES_IN_VOLUMES task removes the subdirectory,
//	                          which is what every other environment here deploys
//	useDeleteFilesTask=false  the driver renames the subdirectory to
//	                          "<driverName>_delete_<subdir>" through its own client mount
//	                          and a background sweep removes the marked directories later
//
// This package is the second path, which no e2e environment had ever deployed. It asserts
// the rename: after the claim is gone the subdirectory is no longer under its own name, and
// a delete marker holding its contents is there instead.
//
// What it deliberately does not assert is the sweep that finally removes the marker.
// LaunchDirectoryDeleter (src/driver/shared_volume_directory_deleter.go) only walks the
// volumes listed in --shared_volumes_list, which takes volume *UUIDs* -- and the UUID of a
// volume created for this run cannot be known when the helm values are set -- and it first
// runs five minutes after the controller starts. Covering that needs a volume that outlives
// the run with its UUID pinned in the env file.
func TestSharedVolumeCleanupRenamesSubdirectory(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	require.True(t, cfg.SharedVolumeOptions.EnableSharedVolume,
		"this test only makes sense with USE_SHARED_VOLUME set; check this directory's env file")
	require.False(t, cfg.SharedVolumeOptions.UseDeleteFilesTask,
		"this test covers the rename branch of DeleteVolume, so the driver must be deployed with "+
			"quobyte.useDeleteFilesTaskForSharedVolumeCleanup=false and the env file must say "+
			"USE_DELETE_FILES_TASK=false")
	require.True(t, cfg.SharedVolumeOptions.PreCreateSharedVolume,
		"the shared volume has to exist before the first claim: this test mounts its root to see "+
			"what the driver leaves in it, which needs a name it knows in advance")

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-svcleanup-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-svcleanup-sc-%d", suffix)
	}
	pvcName := fmt.Sprintf("e2e-svcleanup-pvc-%d", suffix)
	podName := fmt.Sprintf("e2e-svcleanup-pod-%d", suffix)
	rootPVName := fmt.Sprintf("e2e-svcleanup-root-pv-%d", suffix)
	rootPVCName := fmt.Sprintf("e2e-svcleanup-root-pvc-%d", suffix)
	rootPodName := fmt.Sprintf("e2e-svcleanup-root-pod-%d", suffix)

	sharedVolumeName := cfg.SharedVolumeOptions.Name
	if sharedVolumeName == "" {
		sharedVolumeName = fmt.Sprintf("e2e-svcleanup-vol-%d", suffix)
	}

	// Registered before everything else, so LIFO cleanup gives the volume back last -- once
	// both mounts of it and the subdirectory in it are gone.
	sharedVolumeUUID, err := framework.CreateSharedVolume(quobyteClient, sharedVolumeName, tenantID)
	require.NoError(t, err, "pre-creating shared volume %q", sharedVolumeName)
	t.Logf("pre-created shared volume %s (%s)", sharedVolumeName, sharedVolumeUUID)
	framework.CleanupUnlessFailed(t, fmt.Sprintf("shared volume %s", sharedVolumeName), func() {
		if err := framework.DeleteVolume(quobyteClient, sharedVolumeUUID); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID))

	framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:             storageClassName,
		Provisioner:      cfg.CSIProvisionerName,
		Tenant:           cfg.QuobyteTenant,
		SecretName:       secretName,
		SecretNamespace:  cfg.Namespace,
		SharedVolumeName: sharedVolumeName,
	}))

	// --- the shared volume's root, mounted directly ---------------------------
	// A two-part handle mounts the volume itself rather than a subdirectory of it (see
	// processVolumeHandle in src/driver/node.go), which is the only way to see what the
	// driver does inside it. Retain, so tearing this down leaves the volume alone. The
	// root is world-writable (1777, DefaultSharedVolumeAccessModes) because the driver has
	// to create and remove subdirectories in it, so the test's own user may read it.
	framework.ApplyPV(t, ctx, clientset, framework.NewPreProvisionedPV(framework.PVOptions{
		Name:            rootPVName,
		Driver:          cfg.CSIProvisionerName,
		VolumeHandle:    cfg.QuobyteTenant + "|" + sharedVolumeName,
		Size:            "1Gi",
		SecretName:      secretName,
		SecretNamespace: cfg.Namespace,
	}))
	framework.ApplyPVC(t, ctx, clientset, framework.NewPVCForPV(rootPVCName, cfg.Namespace, rootPVName, "1Gi"))
	_, err = framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, rootPVCName, 3*time.Minute)
	require.NoError(t, err, "waiting for the claim on the shared volume's root to bind")

	framework.ApplyPod(t, ctx, clientset, framework.NewPod(rootPodName, cfg.Namespace, rootPVCName, rootMountPath))
	_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, rootPodName, 2*time.Minute)
	require.NoError(t, err, "waiting for the pod mounting the shared volume's root to run")

	// --- a claim provisioned into that volume ---------------------------------
	framework.ApplyPVC(t, ctx, clientset, framework.NewPVC(pvcName, cfg.Namespace, storageClassName, "1Gi"))
	boundPVC, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc %s to bind", pvcName)

	subDir := subDirectoryOf(t, ctx, clientset, boundPVC.Spec.VolumeName)

	framework.ApplyPod(t, ctx, clientset, framework.NewPod(podName, cfg.Namespace, pvcName, claimMountPath))
	_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, podName, 2*time.Minute)
	require.NoError(t, err, "waiting for pod %s to run", podName)

	// Written so the rename is asked to move a directory with something in it, and so the
	// marker can be shown to still hold the data rather than to be an empty leftover.
	const canaryFile = "canary.txt"
	canaryContent := fmt.Sprintf("svcleanup-%d", suffix)
	require.NoError(t, framework.WriteFileInPod(ctx, restConfig, clientset, cfg.Namespace, podName,
		claimMountPath+"/"+canaryFile, canaryContent), "writing into the claim's subdirectory")

	rootEntries := listDirectory(t, ctx, restConfig, clientset, cfg, rootPodName, rootMountPath)
	require.Contains(t, rootEntries, subDir,
		"the claim's subdirectory %q is not in the shared volume's root (%v), so the two mounts "+
			"are not the same volume and nothing below would mean anything", subDir, rootEntries)

	// --- deleting the claim renames it rather than removing it ----------------
	// The PV going away is what says the driver's DeleteVolume returned; before that the
	// rename may simply not have happened yet.
	require.NoError(t, clientset.CoreV1().Pods(cfg.Namespace).Delete(ctx, podName, metav1.DeleteOptions{}),
		"deleting pod %s", podName)
	require.NoError(t, framework.WaitForPodDeleted(ctx, clientset, cfg.Namespace, podName, 2*time.Minute))
	require.NoError(t, clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Delete(ctx, pvcName, metav1.DeleteOptions{}),
		"deleting pvc %s", pvcName)
	require.NoError(t, framework.WaitForPVCDeleted(ctx, clientset, cfg.Namespace, pvcName, 2*time.Minute))
	require.NoError(t, framework.WaitForPVDeleted(ctx, clientset, boundPVC.Spec.VolumeName, 3*time.Minute),
		"the PV was never removed, so the driver did not finish deleting the claim's subdirectory")

	// DELETE_MARKER_FORMAT in src/driver/shared_volume_directory_deleter.go, with the
	// driver's own name as the prefix -- that prefix is what its sweep matches on, so a
	// marker named anything else would never be collected.
	marker := fmt.Sprintf("%s_delete_%s", cfg.CSIProvisionerName, subDir)

	rootEntries = listDirectory(t, ctx, restConfig, clientset, cfg, rootPodName, rootMountPath)
	require.NotContains(t, rootEntries, subDir,
		"the claim is gone but its subdirectory %q is still under its own name in the shared volume (%v)",
		subDir, rootEntries)
	require.Contains(t, rootEntries, marker,
		"the claim's subdirectory was removed without leaving the delete marker %q the driver's "+
			"own sweep looks for; the shared volume now holds %v", marker, rootEntries)

	// The rename moved the directory, it did not empty it: the sweep is what deletes the
	// data, and here it has not run.
	markedContent, err := framework.ReadFileInPod(ctx, restConfig, clientset, cfg.Namespace, rootPodName,
		rootMountPath+"/"+marker+"/"+canaryFile)
	require.NoError(t, err, "reading the canary file back from the delete marker %q", marker)
	require.Equal(t, canaryContent, strings.TrimSpace(markedContent),
		"the delete marker does not hold what the claim's subdirectory held")

	// The shared volume itself is untouched by all of this: only the subdirectory was ever
	// the claim's.
	volumeNames, err := framework.ListVolumeNamesInTenant(quobyteClient, tenantID)
	require.NoError(t, err, "listing the volumes of tenant %s", cfg.QuobyteTenant)
	require.Equal(t, []string{sharedVolumeName}, volumeNames,
		"deleting the claim disturbed the shared volume itself, not just its subdirectory")

	// Left where the driver's sweep would find it, so cleanup does not have to pretend the
	// marker is not there.
	t.Logf("shared volume %s holds the delete marker %s, awaiting the driver's own sweep", sharedVolumeName, marker)
}

// subDirectoryOf returns the subdirectory part of a shared volume PV's handle,
// "<tenant>|<volume>|<subDir>" (see CreateVolume in src/driver/controller.go).
func subDirectoryOf(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, pvName string) string {
	t.Helper()

	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	require.NoError(t, err, "fetching PV %s", pvName)
	require.NotNil(t, pv.Spec.CSI, "PV %s has no CSI volume source", pvName)

	parts := strings.Split(pv.Spec.CSI.VolumeHandle, "|")
	require.Len(t, parts, 3,
		"PV %s was provisioned through a shared volume but its handle %q names no subdirectory",
		pvName, pv.Spec.CSI.VolumeHandle)

	return parts[2]
}

// listDirectory returns the names directly under path, as seen from inside the given pod.
// "ls -A" rather than a bare "ls": the driver's delete markers are not hidden, but a stray
// dot-file appearing in a listing the test compares against would be worth seeing.
func listDirectory(t *testing.T, ctx context.Context, restConfig *rest.Config,
	clientset *kubernetes.Clientset, cfg framework.Config, podName, path string) []string {
	t.Helper()

	stdout, stderr, err := framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, podName,
		framework.TestContainerName, []string{"ls", "-A", "-1", path})
	require.NoErrorf(t, err, "listing %s inside pod %s: stderr=%s", path, podName, stderr)

	entries := []string{}
	for _, line := range strings.Split(stdout, "\n") {
		if entry := strings.TrimSpace(line); entry != "" {
			entries = append(entries, entry)
		}
	}

	return entries
}
