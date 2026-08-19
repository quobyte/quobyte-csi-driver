package sharedvolume

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/e2e-tests/framework"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const mountPath = "/mnt/test"

// TestSharedVolumeProvisionsSubdirectories covers the "Shared volume (existing/not
// existing)" part of that entry in the TODO list of
// e2e-tests/sanity/README.md.
//
// With sharedVolumeName in the StorageClass the driver stops creating a volume per PVC and
// creates a subdirectory of one volume instead, so the PV handle grows a third part,
// "<tenant>|<volume>|<subdir>" (see CreateVolume in src/driver/controller.go). Whether that
// volume already exists or the driver has to create it is what this directory's two env
// files vary; the test is the same either way, which is the point -- the outcome must not
// depend on who created the volume.
//
// Both env files run with USE_DELETE_FILES_TASK=true (the chart default), so deleting a
// claim's PV must not erase the shared volume: DeleteVolume schedules a
// DELETE_FILES_IN_VOLUMES task against the claim's subdirectory instead of taking the
// two-part "erase the whole volume" branch (see DeleteVolume in src/driver/controller.go).
// That the task actually gets scheduled is what this test checks; that it is ever collected
// is not -- the collector only walks volumes named in --shared_volumes_list, which takes
// volume UUIDs, and the UUID of a per-run volume cannot be known when the helm values are
// set. See the note in e2e-tests/sanity/README.md.
func TestSharedVolumeProvisionsSubdirectories(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	require.True(t, cfg.SharedVolumeOptions.EnableSharedVolume,
		"this test only makes sense with USE_SHARED_VOLUME set; check this directory's env files")
	require.True(t, cfg.SharedVolumeOptions.UseDeleteFilesTask,
		"this test covers the delete files task branch of shared volume cleanup; check this directory's env files (USE_DELETE_FILES_TASK)")

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-shared-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-shared-sc-%d", suffix)
	}

	sharedVolume := cfg.SharedVolumeOptions
	if sharedVolume.Name == "" {
		sharedVolume.Name = fmt.Sprintf("e2e-shared-vol-%d", suffix)
	}

	// Either the test brings the volume or the driver does. Registered before everything
	// else so LIFO cleanup removes it last, once both claims and their subdirectories are
	// gone -- and only when this test created it, since a volume the driver made is the
	// driver's to erase.
	if sharedVolume.PreCreateSharedVolume {
		sharedVolumeUUID, err := framework.CreateSharedVolume(quobyteClient, sharedVolume.Name, tenantID)
		require.NoError(t, err, "pre-creating shared volume %q", sharedVolume.Name)
		t.Logf("pre-created shared volume %s (%s)", sharedVolume.Name, sharedVolumeUUID)
		framework.CleanupUnlessFailed(t, fmt.Sprintf("shared volume %s", sharedVolume.Name), func() {
			if err := framework.DeleteVolume(quobyteClient, sharedVolumeUUID); err != nil {
				t.Logf("warning: %v", err)
			}
		})
	}

	suite := &sharedVolumeSuite{t: t, ctx: ctx, cfg: cfg, clientset: clientset, restConfig: restConfig}

	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID))

	framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:             storageClassName,
		Provisioner:      cfg.CSIProvisionerName,
		Tenant:           cfg.QuobyteTenant,
		SecretName:       secretName,
		SecretNamespace:  cfg.Namespace,
		SharedVolumeName: sharedVolume.Name,
	}))

	// --- two claims through that one StorageClass -----------------------------
	first := suite.newClaimWithPod(storageClassName, fmt.Sprintf("a-%d", suffix))
	second := suite.newClaimWithPod(storageClassName, fmt.Sprintf("b-%d", suffix))

	firstTenant, firstVolume, firstSubDir := suite.volumeHandleOf(first.pvcName)
	secondTenant, secondVolume, secondSubDir := suite.volumeHandleOf(second.pvcName)

	require.Equal(t, firstTenant, secondTenant, "the two claims landed in different tenants")
	require.Equal(t, firstVolume, secondVolume,
		"the two claims got a volume each, so the sharedVolumeName parameter did not take effect")
	require.NotEmpty(t, firstSubDir,
		"the PV handle names no subdirectory, so the claim was not provisioned into the shared volume")
	require.NotEqual(t, firstSubDir, secondSubDir,
		"both claims were given the same subdirectory of the shared volume")

	// --- the subdirectories are separate --------------------------------------
	firstContent := fmt.Sprintf("first-%d", suffix)
	suite.writeFile(first.podName, "own.txt", firstContent)
	suite.writeFile(second.podName, "own.txt", fmt.Sprintf("second-%d", suffix))

	require.Equal(t, firstContent, suite.readFile(first.podName, "own.txt"),
		"the first claim's mount shows what was written through the second one, so both mounted the same directory")

	// --- deleting one claim leaves the other, and the volume, alone -----------
	suite.deleteClaimWithPod(second)

	require.Equal(t, firstContent, suite.readFile(first.podName, "own.txt"),
		"deleting the second claim disturbed the first claim's data in the shared volume")

	// ... and deleting the claim's PV has to have scheduled a DELETE_FILES_IN_VOLUMES task
	// for its subdirectory, rather than removing the subdirectory directly.
	scheduled, err := framework.DeleteFilesTaskScheduled(quobyteClient, secondVolume, "/"+secondSubDir)
	require.NoError(t, err, "listing delete-files tasks")
	require.True(t, scheduled,
		"deleting the second claim's PV did not schedule a delete files task for its subdirectory %q", secondSubDir)

	suite.deleteClaimWithPod(first)
	scheduled, err = framework.DeleteFilesTaskScheduled(quobyteClient, firstVolume, "/"+firstSubDir)
	require.NoError(t, err, "listing delete-files tasks")
	require.True(t, scheduled,
		"deleting the first claim's PV did not schedule a delete files task for its subdirectory %q", firstSubDir)

	// The volume itself still has to resolve -- DeleteVolume for a "tenant|volume|subdir"
	// handle must not take the two-part branch that erases the whole volume.
	_, err = quobyteClient.ResolveVolumeNameToUUID(sharedVolume.Name, tenantID)
	require.NoError(t, err, "deleting a claim removed the shared volume itself, not just its subdirectory")
}

// sharedVolumeSuite carries what every step below needs, so the steps read as what they do.
type sharedVolumeSuite struct {
	t          *testing.T
	ctx        context.Context
	cfg        framework.Config
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
}

// claimWithPod is one PVC and the pod mounting it.
type claimWithPod struct {
	pvcName string
	podName string
}

func (s *sharedVolumeSuite) newClaimWithPod(storageClassName, nameSuffix string) claimWithPod {
	s.t.Helper()

	claim := claimWithPod{
		pvcName: fmt.Sprintf("e2e-shared-pvc-%s", nameSuffix),
		podName: fmt.Sprintf("e2e-shared-pod-%s", nameSuffix),
	}

	framework.ApplyPVC(s.t, s.ctx, s.clientset,
		framework.NewPVC(claim.pvcName, s.cfg.Namespace, storageClassName, "1Gi"))
	_, err := framework.WaitForPVCBound(s.ctx, s.clientset, s.cfg.Namespace, claim.pvcName, 3*time.Minute)
	require.NoError(s.t, err, "waiting for pvc %s to bind", claim.pvcName)

	framework.ApplyPod(s.t, s.ctx, s.clientset,
		framework.NewPod(claim.podName, s.cfg.Namespace, claim.pvcName, mountPath))
	_, err = framework.WaitForPodRunning(s.ctx, s.clientset, s.cfg.Namespace, claim.podName, 2*time.Minute)
	require.NoError(s.t, err, "waiting for pod %s to run", claim.podName)

	return claim
}

// deleteClaimWithPod removes one claim and its pod mid-test, waiting for both to be gone so
// that what follows observes the state after the deletion rather than during it. Its own
// cleanup registration is left in place: deleting something twice is not an error.
func (s *sharedVolumeSuite) deleteClaimWithPod(claim claimWithPod) {
	s.t.Helper()

	require.NoError(s.t, s.clientset.CoreV1().Pods(s.cfg.Namespace).Delete(s.ctx, claim.podName, metav1.DeleteOptions{}),
		"deleting pod %s", claim.podName)
	require.NoError(s.t, framework.WaitForPodDeleted(s.ctx, s.clientset, s.cfg.Namespace, claim.podName, 2*time.Minute))

	require.NoError(s.t, s.clientset.CoreV1().PersistentVolumeClaims(s.cfg.Namespace).Delete(s.ctx, claim.pvcName, metav1.DeleteOptions{}),
		"deleting pvc %s", claim.pvcName)
	require.NoError(s.t, framework.WaitForPVCDeleted(s.ctx, s.clientset, s.cfg.Namespace, claim.pvcName, 2*time.Minute))
}

// volumeHandleOf returns the tenant, volume and subdirectory the claim's PV was given.
func (s *sharedVolumeSuite) volumeHandleOf(pvcName string) (tenant, volume, subDir string) {
	s.t.Helper()

	pvc, err := s.clientset.CoreV1().PersistentVolumeClaims(s.cfg.Namespace).Get(s.ctx, pvcName, metav1.GetOptions{})
	require.NoError(s.t, err, "fetching pvc %s", pvcName)

	pv, err := s.clientset.CoreV1().PersistentVolumes().Get(s.ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
	require.NoError(s.t, err, "fetching the PV bound to %s", pvcName)
	require.NotNil(s.t, pv.Spec.CSI, "PV %s has no CSI volume source", pv.Name)

	parts := strings.Split(pv.Spec.CSI.VolumeHandle, "|")
	require.GreaterOrEqual(s.t, len(parts), 2, "unexpected volume handle format %q", pv.Spec.CSI.VolumeHandle)
	if len(parts) > 2 {
		subDir = parts[2]
	}

	return parts[0], parts[1], subDir
}

func (s *sharedVolumeSuite) writeFile(podName, fileName, content string) {
	s.t.Helper()

	require.NoError(s.t, framework.WriteFileInPod(s.ctx, s.restConfig, s.clientset, s.cfg.Namespace,
		podName, mountPath+"/"+fileName, content))
}

func (s *sharedVolumeSuite) readFile(podName, fileName string) string {
	s.t.Helper()

	content, err := framework.ReadFileInPod(s.ctx, s.restConfig, s.clientset, s.cfg.Namespace,
		podName, mountPath+"/"+fileName)
	require.NoError(s.t, err)

	return content
}
