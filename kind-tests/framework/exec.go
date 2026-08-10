package framework

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecInPod runs cmd inside the given pod/container and returns its captured
// stdout/stderr, using the same mechanism as `kubectl exec`.
func ExecInPod(ctx context.Context, restConfig *rest.Config, clientset *kubernetes.Clientset, namespace, pod, container string, cmd []string) (stdout, stderr string, err error) {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   cmd,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("creating executor: %w", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	})
	if err != nil {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("streaming exec %v: %w", cmd, err)
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}
