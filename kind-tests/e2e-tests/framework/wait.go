package framework

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const pollInterval = 5 * time.Second

// WaitForPVCBound polls until the named PVC reaches the Bound phase or the
// timeout elapses.
func WaitForPVCBound(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) (*corev1.PersistentVolumeClaim, error) {
	var bound *corev1.PersistentVolumeClaim

	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		pvc, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pvc.Status.Phase == corev1.ClaimBound {
			bound = pvc
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("waiting for PVC %s/%s to be bound: %w", namespace, name, err)
	}

	return bound, nil
}

// WaitForPodRunning polls until the named pod reaches the Running phase or
// the timeout elapses.
func WaitForPodRunning(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) (*corev1.Pod, error) {
	var running *corev1.Pod

	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodRunning {
			running = pod
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("waiting for pod %s/%s to be running: %w", namespace, name, err)
	}

	return running, nil
}
