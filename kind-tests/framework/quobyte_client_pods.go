package framework

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Where run_test's quobyte-client helm release puts its DaemonSet, and the label that
// selects its pods (the same one kind-tests/run_test collects client logs with).
const (
	QuobyteClientNamespace     = "kube-system"
	QuobyteClientPodLabel      = "role=client"
	quobyteClientNodeFieldName = "spec.nodeName"
)

// QuobyteClientPodOnNode returns the quobyte-client pod serving mounts on the given node.
// The client runs as a DaemonSet, so there is exactly one; a test that needs to disturb a
// pod's mount has to disturb the client on that pod's own node.
func QuobyteClientPodOnNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName string) (*corev1.Pod, error) {
	pods, err := clientset.CoreV1().Pods(QuobyteClientNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: QuobyteClientPodLabel,
		FieldSelector: quobyteClientNodeFieldName + "=" + nodeName,
	})
	if err != nil {
		return nil, fmt.Errorf("listing quobyte-client pods on node %s: %w", nodeName, err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no quobyte-client pod (%s) found on node %s", QuobyteClientPodLabel, nodeName)
	}

	return &pods.Items[0], nil
}
