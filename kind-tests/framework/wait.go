package framework

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// waitForGone polls get until it reports the object is gone. Deleting a k8s
// object only marks it for deletion, so a test that tears its resources down in
// a specific order has to wait for each one to actually disappear before moving
// on to the one it depends on.
func waitForGone(ctx context.Context, timeout time.Duration, description string, get func(context.Context) error) error {
	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		err := get(ctx)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		// Either the object is still there (err == nil) or the API call itself
		// failed, in which case polling stops and reports that error.
		return false, err
	})
	if err != nil {
		return fmt.Errorf("waiting for %s to be deleted: %w", description, err)
	}

	return nil
}

// WaitForPodDeleted polls until the named pod is gone or the timeout elapses.
func WaitForPodDeleted(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	return waitForGone(ctx, timeout, fmt.Sprintf("pod %s/%s", namespace, name), func(ctx context.Context) error {
		_, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	})
}

// WaitForPVCDeleted polls until the named PVC is gone or the timeout elapses.
func WaitForPVCDeleted(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	return waitForGone(ctx, timeout, fmt.Sprintf("pvc %s/%s", namespace, name), func(ctx context.Context) error {
		_, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	})
}

// WaitForPVDeleted polls until the named PV is gone or the timeout elapses. For
// a dynamically provisioned volume with reclaimPolicy Delete this is what shows
// the CSI driver finished deleting the backing Quobyte volume -- until then, the
// credentials it uses for that must stay in place.
func WaitForPVDeleted(ctx context.Context, clientset *kubernetes.Clientset, name string, timeout time.Duration) error {
	return waitForGone(ctx, timeout, fmt.Sprintf("pv %s", name), func(ctx context.Context) error {
		_, err := clientset.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
		return err
	})
}

// WaitForStorageClassDeleted polls until the named StorageClass is gone or the
// timeout elapses.
func WaitForStorageClassDeleted(ctx context.Context, clientset *kubernetes.Clientset, name string, timeout time.Duration) error {
	return waitForGone(ctx, timeout, fmt.Sprintf("storage class %s", name), func(ctx context.Context) error {
		_, err := clientset.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
		return err
	})
}

// WaitForSecretDeleted polls until the named Secret is gone or the timeout
// elapses.
func WaitForSecretDeleted(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	return waitForGone(ctx, timeout, fmt.Sprintf("secret %s/%s", namespace, name), func(ctx context.Context) error {
		_, err := clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	})
}
