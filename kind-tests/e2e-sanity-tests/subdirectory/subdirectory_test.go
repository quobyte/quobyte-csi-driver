package subdirectory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
)

const mountPath = "/mnt/test"

// The driver accepts a three-part volume handle, "<tenant>|<volume>|<subdir>", and mounts
// that subdirectory rather than the volume root (formMountPath in src/driver/node.go).
// shared_volume covers the handles the provisioner produces; this covers the ones written
// by hand, pointing a PV at a subdirectory of a volume that already exists.
//
// Two PVs are pointed at the same Quobyte volume at once -- one at its root, one at a
// subdirectory of it -- which is what makes the containment assertions possible: the API
// has no file operations, so the root mount is also how the subdirectory gets created and
// filled in the first place.
func TestSubdirectoryOfExistingVolumeIsMountable(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	volumeName := fmt.Sprintf("e2e-subdir-vol-%d", suffix)
	secretName := fmt.Sprintf("e2e-subdir-secret-%d", suffix)
	const subDirName = "subdir"

	// The shared volume access mode: the root mount has to be able to create a directory
	// in it, which the default 700 would allow only for the owner.
	volumeUUID, err := framework.CreateVolume(quobyteClient, volumeName, tenantID, framework.SharedVolumeAccessMode)
	require.NoError(t, err, "creating quobyte volume %q", volumeName)
	t.Logf("created quobyte volume %s (%s) in tenant %s", volumeName, volumeUUID, cfg.QuobyteTenant)
	framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte volume %s", volumeName), func() {
		if err := framework.DeleteVolume(quobyteClient, volumeUUID); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	secret := framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID)
	framework.ApplySecret(t, ctx, clientset, secret)

	// --- a pod on the volume root, to create and fill the subdirectory ---------
	rootPodName := mountVolume(t, ctx, clientset, cfg, secretName,
		cfg.QuobyteTenant+"|"+volumeName, fmt.Sprintf("root-%d", suffix))

	_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, rootPodName, 2*time.Minute)
	require.NoError(t, err, "waiting for the pod mounting the volume root to run")

	subDirContent := fmt.Sprintf("in-subdir-%d", suffix)
	rootContent := fmt.Sprintf("in-root-%d", suffix)
	_, stderr, err := framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, rootPodName, "test-container",
		[]string{"sh", "-c", fmt.Sprintf("mkdir -p %s/%s && echo -n %s > %s/%s/inside.txt && echo -n %s > %s/outside.txt",
			mountPath, subDirName, subDirContent, mountPath, subDirName, rootContent, mountPath)})
	require.NoErrorf(t, err, "creating and filling the subdirectory through the volume root: stderr=%s", stderr)

	// --- a second pod on the subdirectory of that same volume ------------------
	subDirPodName := mountVolume(t, ctx, clientset, cfg, secretName,
		cfg.QuobyteTenant+"|"+volumeName+"|"+subDirName, fmt.Sprintf("sub-%d", suffix))

	_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, subDirPodName, 2*time.Minute)
	require.NoError(t, err, "waiting for the pod mounting the subdirectory to run")

	// What the subdirectory holds is at the root of this mount, not one level down.
	stdout, stderr, err := framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, subDirPodName, "test-container",
		[]string{"cat", mountPath + "/inside.txt"})
	require.NoErrorf(t, err, "reading the subdirectory's file through the subdirectory mount: stderr=%s", stderr)
	require.Equal(t, subDirContent, strings.TrimSpace(stdout),
		"the subdirectory mount does not show the content written into that subdirectory")

	// And nothing above it is: a mount of the volume root would show this file.
	_, _, err = framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, subDirPodName, "test-container",
		[]string{"cat", mountPath + "/outside.txt"})
	require.Error(t, err,
		"the subdirectory mount exposes a file that lives in the volume root, so it mounted the volume rather than the subdirectory")

	// Writes through the subdirectory mount land in the subdirectory, which the root mount
	// sees one level down.
	writtenBack := fmt.Sprintf("written-through-subdir-%d", suffix)
	_, stderr, err = framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, subDirPodName, "test-container",
		[]string{"sh", "-c", fmt.Sprintf("echo -n %s > %s/back.txt", writtenBack, mountPath)})
	require.NoErrorf(t, err, "writing through the subdirectory mount: stderr=%s", stderr)

	stdout, stderr, err = framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, rootPodName, "test-container",
		[]string{"cat", fmt.Sprintf("%s/%s/back.txt", mountPath, subDirName)})
	require.NoErrorf(t, err, "reading the subdirectory write back through the volume root: stderr=%s", stderr)
	require.Equal(t, writtenBack, strings.TrimSpace(stdout),
		"a write through the subdirectory mount did not land in that subdirectory of the volume")
}

// mountVolume creates the PV/PVC/pod trio that mounts one volume handle, and returns the
// pod's name. The PV is reclaimed by Retain, so none of this can take the underlying
// Quobyte volume away when it is torn down.
func mountVolume(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, cfg framework.Config, secretName, volumeHandle, nameSuffix string) string {
	t.Helper()

	pvName := fmt.Sprintf("e2e-subdir-pv-%s", nameSuffix)
	pvcName := fmt.Sprintf("e2e-subdir-pvc-%s", nameSuffix)
	podName := fmt.Sprintf("e2e-subdir-pod-%s", nameSuffix)

	framework.ApplyPV(t, ctx, clientset, framework.NewPreProvisionedPV(framework.PVOptions{
		Name:            pvName,
		Driver:          cfg.CSIProvisionerName,
		VolumeHandle:    volumeHandle,
		Size:            "1Gi",
		SecretName:      secretName,
		SecretNamespace: cfg.Namespace,
	}))
	framework.ApplyPVC(t, ctx, clientset, framework.NewPVCForPV(pvcName, cfg.Namespace, pvName, "1Gi"))

	_, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc %s to bind to pv %s", pvcName, pvName)

	framework.ApplyPod(t, ctx, clientset, framework.NewPod(podName, cfg.Namespace, pvcName, mountPath))

	return podName
}
