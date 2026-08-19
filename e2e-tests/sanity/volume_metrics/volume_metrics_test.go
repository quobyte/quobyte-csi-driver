package volumemetrics

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/e2e-tests/framework"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	mountPath = "/mnt/test"
	// The kubelet collects volume stats on a timer, so the series do not appear the moment
	// the pod runs. Long enough for a couple of collection rounds; the disabled case waits
	// the same time before concluding that nothing will appear.
	metricsTimeout = 3 * time.Minute
	// One of the series the kubelet exports per claim from what NodeGetVolumeStats returns.
	// Capacity rather than used bytes: it is non-zero for an empty volume too.
	capacityMetric = "kubelet_volume_stats_capacity_bytes"
)

// TestVolumeMetricsFollowTheDriverFlag covers quobyte.enableVolumeMetrics and the
// NodeGetVolumeStats implementation behind it (src/driver/node.go), neither of which was
// asserted anywhere. Metrics are on by default, so a regression that broke them -- or one
// that made the disabled driver fail mounts instead of only refusing stats -- would have
// reached a release unnoticed.
//
// The two env files are the two settings of the flag, and the test asserts both directions
// against the same evidence, the kubelet's own /metrics:
//
//	enabled   the kubelet exports kubelet_volume_stats_* for the claim
//	disabled  it exports none for it, and the volume is mounted and writable all the same
//
// The absence in the second case is only meaningful because the first case shows the series
// do appear within metricsTimeout on an otherwise identical cluster.
func TestVolumeMetricsFollowTheDriverFlag(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-metrics-secret-%d", suffix)
	storageClassName := cfg.StorageClassName
	if storageClassName == "" {
		storageClassName = fmt.Sprintf("e2e-metrics-sc-%d", suffix)
	}
	pvcName := fmt.Sprintf("e2e-metrics-pvc-%d", suffix)
	podName := fmt.Sprintf("e2e-metrics-pod-%d", suffix)

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

	framework.ApplyPod(t, ctx, clientset, framework.NewPod(podName, cfg.Namespace, pvcName, mountPath))
	pod, err := framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, podName, 3*time.Minute)
	require.NoError(t, err, "waiting for pod to run")

	// Asserted in both directions: whether stats are exported or refused, the volume itself
	// has to work. A driver that failed mounts when metrics are off would be the worse bug
	// of the two.
	content := fmt.Sprintf("metrics-%d", suffix)
	require.NoError(t, framework.WriteFileInPod(ctx, restConfig, clientset, cfg.Namespace, podName,
		mountPath+"/metrics.txt", content), "writing into the mounted volume")
	got, err := framework.ReadFileInPod(ctx, restConfig, clientset, cfg.Namespace, podName, mountPath+"/metrics.txt")
	require.NoError(t, err, "reading back from the mounted volume")
	require.Equal(t, content, strings.TrimSpace(got), "the mount is not writable")

	// The claim's series live on the kubelet of the node its pod runs on.
	node := pod.Spec.NodeName
	require.NotEmpty(t, node, "the running pod names no node")

	found, sample := waitForClaimMetric(ctx, clientset, node, cfg.Namespace, pvcName)

	if cfg.EnableVolumeMetrics {
		require.True(t, found,
			"the driver was deployed with volume metrics on, but the kubelet on %s exported no %s "+
				"for claim %s within %s", node, capacityMetric, pvcName, metricsTimeout)
		t.Logf("kubelet on %s exports: %s", node, sample)

		return
	}

	require.False(t, found,
		"the driver was deployed with quobyte.enableVolumeMetrics=false, so NodeGetVolumeStats "+
			"refuses every request -- yet the kubelet on %s still exports %s for claim %s: %s",
		node, capacityMetric, pvcName, sample)
	t.Logf("no %s for claim %s on %s, and the volume is mounted and writable", capacityMetric, pvcName, node)
}

// waitForClaimMetric polls the kubelet's /metrics on the given node until it exports the
// capacity series for this claim, and returns the sample line it found. It always waits the
// full metricsTimeout when nothing appears, which is what makes a negative answer worth
// something: the disabled case concludes from it.
func waitForClaimMetric(ctx context.Context, clientset *kubernetes.Clientset, node, namespace, pvcName string) (bool, string) {
	want := fmt.Sprintf("persistentvolumeclaim=%q", pvcName)
	var sample string

	// Errors reading the kubelet are treated as "not yet": the endpoint is up long before
	// the volume stats collector has run, but a transient failure should not decide the
	// test either way.
	_ = wait.PollUntilContextTimeout(ctx, 10*time.Second, metricsTimeout, true, func(ctx context.Context) (bool, error) {
		raw, err := clientset.CoreV1().RESTClient().Get().
			AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/metrics", node)).
			DoRaw(ctx)
		if err != nil {
			return false, nil
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, capacityMetric) && strings.Contains(line, want) &&
				strings.Contains(line, fmt.Sprintf("namespace=%q", namespace)) {
				sample = strings.TrimSpace(line)
				return true, nil
			}
		}
		return false, nil
	})

	return sample != "", sample
}
