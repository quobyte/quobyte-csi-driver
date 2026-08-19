package multinodeaccess

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/e2e-tests/framework"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const mountPath = "/mnt/test"

// TestOneVolumeMountedFromTwoNodes covers ReadWriteMany, which the driver declares to the
// upstream suite (RWX: true in e2e-tests/e2e) but which nothing in this suite had ever
// exercised: every claim the framework builds is ReadWriteOnce, and a scheduler is free to
// put two such pods on the same node, where a shared mount proves nothing.
//
// A Quobyte volume is a shared file system, so mounting it from several nodes at once is the
// normal case rather than an edge case -- and the node plugin mounts through the node's
// Quobyte client, so nothing about it is per-node state that Kubernetes tracks. This test
// pins one pod to each of two workers and has them read what the other wrote, in both
// directions, which is the part a customer's ReadWriteMany workload depends on.
func TestOneVolumeMountedFromTwoNodes(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	firstNode, secondNode := twoWorkerNodes(t, ctx, clientset)
	t.Logf("mounting one volume from %s and %s", firstNode, secondNode)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-multinode-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-multinode-sc-%d", suffix)
	}
	pvcName := fmt.Sprintf("e2e-multinode-pvc-%d", suffix)
	firstPodName := fmt.Sprintf("e2e-multinode-pod-a-%d", suffix)
	secondPodName := fmt.Sprintf("e2e-multinode-pod-b-%d", suffix)

	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID))

	framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:            storageClassName,
		Provisioner:     cfg.CSIProvisionerName,
		Tenant:          cfg.QuobyteTenant,
		SecretName:      secretName,
		SecretNamespace: cfg.Namespace,
	}))

	// The one claim both pods mount. ReadWriteOnce would be enough for Kubernetes to admit
	// two pods on two nodes in some versions, and refuse it in others -- asking for what the
	// volume actually is takes that out of the picture.
	framework.ApplyPVC(t, ctx, clientset, framework.NewPVCWithAccessModes(
		pvcName, cfg.Namespace, storageClassName, "1Gi", corev1.ReadWriteMany))
	_, err = framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for the ReadWriteMany claim to bind")

	// Pinned rather than scheduled: two pods of the same shape are free to land on the same
	// node, and then the test would pass without ever crossing a node boundary.
	framework.ApplyPod(t, ctx, clientset,
		framework.NewPodOnNode(firstPodName, cfg.Namespace, pvcName, mountPath, firstNode))
	framework.ApplyPod(t, ctx, clientset,
		framework.NewPodOnNode(secondPodName, cfg.Namespace, pvcName, mountPath, secondNode))

	firstPod, err := framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, firstPodName, 3*time.Minute)
	require.NoError(t, err, "waiting for the pod on %s to run", firstNode)
	secondPod, err := framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, secondPodName, 3*time.Minute)
	require.NoError(t, err, "waiting for the pod on %s to run", secondNode)

	require.NotEqual(t, firstPod.Spec.NodeName, secondPod.Spec.NodeName,
		"both pods ended up on %s, so the volume was never mounted from two nodes", firstPod.Spec.NodeName)

	// --- what one writes, the other sees, and the other way round --------------
	fromFirst := fmt.Sprintf("from-%s-%d", firstNode, suffix)
	require.NoError(t, framework.WriteFileInPod(ctx, restConfig, clientset, cfg.Namespace, firstPodName,
		mountPath+"/from-first.txt", fromFirst), "writing from the pod on %s", firstNode)

	got, err := framework.ReadFileInPod(ctx, restConfig, clientset, cfg.Namespace, secondPodName,
		mountPath+"/from-first.txt")
	require.NoError(t, err, "reading on %s what was written on %s", secondNode, firstNode)
	require.Equal(t, fromFirst, strings.TrimSpace(got),
		"the pod on %s does not see what the pod on %s wrote, so the two mounts are not the same volume",
		secondNode, firstNode)

	fromSecond := fmt.Sprintf("from-%s-%d", secondNode, suffix)
	require.NoError(t, framework.WriteFileInPod(ctx, restConfig, clientset, cfg.Namespace, secondPodName,
		mountPath+"/from-second.txt", fromSecond), "writing from the pod on %s", secondNode)

	got, err = framework.ReadFileInPod(ctx, restConfig, clientset, cfg.Namespace, firstPodName,
		mountPath+"/from-second.txt")
	require.NoError(t, err, "reading on %s what was written on %s", firstNode, secondNode)
	require.Equal(t, fromSecond, strings.TrimSpace(got),
		"the pod on %s does not see what the pod on %s wrote", firstNode, secondNode)
}

// twoWorkerNodes returns the names of two nodes that carry workloads. The control plane is
// left out: a pod pinned to it bypasses the scheduler and lands on a node the cluster does
// not mean to run workloads on, which is a different thing from what this test is about.
func twoWorkerNodes(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset) (string, string) {
	t.Helper()

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: "!node-role.kubernetes.io/control-plane",
	})
	require.NoError(t, err, "listing the cluster's worker nodes")

	workers := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		workers = append(workers, node.Name)
	}

	require.GreaterOrEqualf(t, len(workers), 2,
		"this test mounts one volume from two nodes at once and the cluster has %d worker node(s): %v",
		len(workers), workers)

	return workers[0], workers[1]
}
