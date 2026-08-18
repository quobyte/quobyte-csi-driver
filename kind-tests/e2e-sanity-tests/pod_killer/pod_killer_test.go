package podkiller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	mountPath = "/mnt/test"
	// Deleting the client pod, the monitor noticing, and the DaemonSet putting a client
	// back so the replacement pod can mount again all happen at their own pace, so these
	// are deliberately generous -- the test is about whether it happens at all.
	podKillTimeout    = 5 * time.Minute
	podRestartTimeout = 5 * time.Minute
)

// TestPodKillerRestartsPodsWithStaleMounts covers the "Pod killer tests" entry of the TODO
// list in kind-tests/e2e-sanity-tests/README.md.
//
// The pod killer (kind-tests/quobyte-csi-pod-killer, deployed by the quobyte-csi chart and
// built from source by test_runner) walks the kubelet's CSI mount paths and treats a mount as
// stale when getxattr(quobyte.statuspage_port) on it returns ENOTCONN -- which is what
// happens once the Quobyte client serving that mount is gone. It then deletes the pod, so
// whatever owns the pod can reschedule it onto a working mount. Deleting is the whole of
// its contract; the recovery afterwards is Kubernetes doing its usual job, and the test
// asserts both because only together do they mean anything to a user.
//
// The mount is made stale by deleting the quobyte-client pod on the node -- which breaks
// every Quobyte mount on that node, acceptable because test_runner's cluster runs nothing else
// against Quobyte.
//
// A Deployment rather than a bare pod: a pod the pod killer deletes would otherwise simply
// stay gone, and there would be no recovery to observe.
func TestPodKillerRestartsPodsWithStaleMounts(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-podkiller-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-podkiller-sc-%d", suffix)
	}
	pvcName := fmt.Sprintf("e2e-podkiller-pvc-%d", suffix)
	deploymentName := fmt.Sprintf("e2e-podkiller-%d", suffix)
	podSelector := "app=" + deploymentName

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
	_, err = framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc to bind")

	framework.ApplyDeployment(t, ctx, clientset,
		framework.NewDeployment(deploymentName, cfg.Namespace, pvcName, mountPath))

	original, err := framework.WaitForRunningPodOtherThan(ctx, clientset, cfg.Namespace, podSelector, "", podRestartTimeout)
	require.NoError(t, err, "waiting for the deployment's pod to run")
	require.NotEmpty(t, original.Spec.NodeName, "the running pod is not scheduled to a node")
	t.Logf("deployment pod %s (uid %s) runs on node %s", original.Name, original.UID, original.Spec.NodeName)

	// Written before the mount breaks, read after the pod comes back: the replacement has
	// to land on a mount that still holds the data, not merely on some mount.
	wantContent := fmt.Sprintf("before-the-mount-broke-%d", suffix)
	require.NoError(t, framework.WriteFileInPod(ctx, restConfig, clientset, cfg.Namespace,
		original.Name, mountPath+"/e2e-test.txt", wantContent))

	// --- break the mount ------------------------------------------------------
	clientPod, err := framework.QuobyteClientPodOnNode(ctx, clientset, original.Spec.NodeName)
	require.NoError(t, err, "finding the quobyte-client pod serving node %s", original.Spec.NodeName)
	t.Logf("deleting quobyte-client pod %s/%s to make the mount stale", clientPod.Namespace, clientPod.Name)

	require.NoError(t, clientset.CoreV1().Pods(clientPod.Namespace).Delete(ctx, clientPod.Name, metav1.DeleteOptions{}),
		"deleting the quobyte-client pod on node %s", original.Spec.NodeName)

	// --- the pod killer acts --------------------------------------------------
	require.NoError(t, framework.WaitForPodGoneByUID(ctx, clientset, cfg.Namespace, original.Name, original.UID, podKillTimeout),
		"the pod kept its stale Quobyte mount: the pod killer did not delete it. Check the pod killer's logs -- "+
			"it resolves pods through its cache service by DNS, and a cache it cannot reach looks exactly like this")

	replacement, err := framework.WaitForRunningPodOtherThan(ctx, clientset, cfg.Namespace, podSelector, original.UID, podRestartTimeout)
	require.NoError(t, err, "waiting for the deployment to replace the killed pod with one that runs")
	t.Logf("pod %s (uid %s) replaced the killed one", replacement.Name, replacement.UID)

	// --- and the replacement has a working mount ------------------------------
	got, err := framework.ReadFileInPod(ctx, restConfig, clientset, cfg.Namespace, replacement.Name, mountPath+"/e2e-test.txt")
	require.NoError(t, err, "reading through the replacement pod's mount")
	require.Equal(t, wantContent, got,
		"the replacement pod's mount does not hold what was written before the mount went stale")
}
