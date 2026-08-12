package framework

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// WriteFileInPod writes content to path inside the pod's test container, which is how a
// test puts data into a Quobyte mount.
func WriteFileInPod(ctx context.Context, restConfig *rest.Config, clientset *kubernetes.Clientset, namespace, pod, path, content string) error {
	_, stderr, err := ExecInPod(ctx, restConfig, clientset, namespace, pod, TestContainerName,
		[]string{"sh", "-c", fmt.Sprintf("echo -n %s > %s", content, path)})
	if err != nil {
		return fmt.Errorf("writing %s in pod %s/%s: %w (stderr=%s)", path, namespace, pod, err, stderr)
	}

	return nil
}

// ReadFileInPod returns the content of path inside the pod's test container, with
// surrounding whitespace trimmed. An error means the file could not be read at all, which
// is itself the expected outcome for a path that should not be visible in a mount.
func ReadFileInPod(ctx context.Context, restConfig *rest.Config, clientset *kubernetes.Clientset, namespace, pod, path string) (string, error) {
	stdout, stderr, err := ExecInPod(ctx, restConfig, clientset, namespace, pod, TestContainerName,
		[]string{"cat", path})
	if err != nil {
		return "", fmt.Errorf("reading %s in pod %s/%s: %w (stderr=%s)", path, namespace, pod, err, stderr)
	}

	return strings.TrimSpace(stdout), nil
}

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
