package preprovisionedvolumes

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const mountPath = "/mnt/test"

// TestPreProvisionedVolumeIsWritable covers the "Pre-provisioned volume tests" entry of the
// TODO list in kind-tests/e2e-sanity-tests/README.md.
//
// Where dynamic_provisioning has the driver create the volume, here nothing ever calls
// CreateVolume: the volume exists first and Kubernetes is pointed at it, so only the node
// plugin's mount path is exercised. There is no StorageClass either -- the claim binds to
// one named PV, which carries the credentials to mount with in its nodePublishSecretRef.
// Mirrors quobyte-k8s-resources/usage-examples/03_using_preprovisioned_volumes_with_access_keys.
//
// The volume is the test's own, not the driver's: the PV is reclaimed by Retain, so
// deleting it must leave the volume alone, which the teardown asserts before removing the
// volume through the API.
func TestPreProvisionedVolumeIsWritable(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	volumeName := fmt.Sprintf("e2e-preprov-vol-%d", suffix)
	secretName := fmt.Sprintf("e2e-preprov-secret-%d", suffix)
	pvName := fmt.Sprintf("e2e-preprov-pv-%d", suffix)
	pvcName := fmt.Sprintf("e2e-preprov-pvc-%d", suffix)
	podName := fmt.Sprintf("e2e-preprov-pod-%d", suffix)

	// --- the credentials, before the volume they have to own -------------------
	// The user is this test's own (framework.CreateTestUser), and the volume below is
	// created owned by it: with the default access mode of 700 nobody else could write to
	// the mount, and the pod at the end of this test does.
	testUser, secret := framework.NewCredentials(t, cfg, quobyteClient, secretName, tenantID)

	// --- the volume, created before Kubernetes knows anything about it ---------
	volumeUUID, err := framework.CreateVolumeOwnedBy(quobyteClient, volumeName, tenantID,
		framework.DefaultVolumeAccessMode, testUser.Name, testUser.PrimaryGroup)
	require.NoError(t, err, "creating quobyte volume %q owned by %s", volumeName, testUser.Name)
	t.Logf("created quobyte volume %s (%s) in tenant %s, owned by %s", volumeName, volumeUUID, cfg.QuobyteTenant, testUser.Name)
	// Registered before the k8s resources below, so LIFO cleanup removes it after the pod,
	// the claim and the PV that referenced it are gone -- and before the user owning it.
	framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte volume %s", volumeName), func() {
		if err := framework.DeleteVolume(quobyteClient, volumeUUID); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	// --- Kubernetes pointed at it ---------------------------------------------
	framework.ApplySecret(t, ctx, clientset, secret)

	// Tenant and volume by name rather than by UUID, which is what makes this readable in
	// a manifest -- the node plugin resolves them through the API credentials in the
	// secret above (src/driver/node.go).
	volumeHandle := cfg.QuobyteTenant + "|" + volumeName
	framework.ApplyPV(t, ctx, clientset, framework.NewPreProvisionedPV(framework.PVOptions{
		Name:            pvName,
		Driver:          cfg.CSIProvisionerName,
		VolumeHandle:    volumeHandle,
		Size:            "1Gi",
		SecretName:      secretName,
		SecretNamespace: cfg.Namespace,
	}))

	framework.ApplyPVC(t, ctx, clientset, framework.NewPVCForPV(pvcName, cfg.Namespace, pvName, "1Gi"))

	boundPVC, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc to bind to the pre-provisioned pv")
	require.Equal(t, pvName, boundPVC.Spec.VolumeName,
		"pvc bound to a different volume than the pre-provisioned one, so it was provisioned dynamically")

	framework.ApplyPod(t, ctx, clientset, framework.NewPod(podName, cfg.Namespace, pvcName, mountPath))

	_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, podName, 2*time.Minute)
	require.NoError(t, err, "waiting for pod to run")

	// --- the mount is the volume that was created above ------------------------
	testFile := mountPath + "/e2e-test.txt"
	wantContent := fmt.Sprintf("e2e-%d", suffix)

	_, stderr, err := framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, podName, "test-container",
		[]string{"sh", "-c", fmt.Sprintf("echo -n %s > %s", wantContent, testFile)})
	require.NoErrorf(t, err, "writing file into the mounted pre-provisioned volume: stderr=%s", stderr)

	stdout, stderr, err := framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, podName, "test-container",
		[]string{"cat", testFile})
	require.NoErrorf(t, err, "reading file back from the mounted pre-provisioned volume: stderr=%s", stderr)
	require.Equal(t, wantContent, strings.TrimSpace(stdout),
		"file content read back from the Quobyte mount did not match what was written")

	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	require.NoError(t, err, "fetching the pre-provisioned PV")
	require.NotNil(t, pv.Spec.CSI, "pre-provisioned PV has no CSI volume source")
	require.Equal(t, volumeHandle, pv.Spec.CSI.VolumeHandle,
		"the PV's volume handle changed, so the mount is not the volume this test created")

	gotUUID, err := quobyteClient.GetVolumeUUID(volumeName, cfg.QuobyteTenant)
	require.NoError(t, err, "resolving the pre-provisioned volume through the Quobyte API")
	require.Equal(t, volumeUUID, gotUUID,
		"the handle resolves to a different volume than the one created for this test")

	// Nothing else may have appeared in the tenant: another volume here would mean the
	// driver provisioned one of its own instead of using the pre-provisioned one.
	volumeNames, err := framework.ListVolumeNamesInTenant(quobyteClient, tenantID)
	require.NoError(t, err, "listing the volumes of tenant %s", cfg.QuobyteTenant)
	require.Equal(t, []string{volumeName}, volumeNames,
		"expected the tenant to hold only the pre-provisioned volume")
}
