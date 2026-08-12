package framework

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

// EnsurePVCStaysUnbound polls the named PVC for the given duration and fails as soon as it
// binds. The inverse of WaitForPVCBound, for the cases where provisioning is supposed to be
// refused: "still Pending" is only meaningful if it stays that way, but a test that just
// slept could not tell "never bound" from "bound a moment later".
func EnsurePVCStaysUnbound(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, duration time.Duration) error {
	err := wait.PollUntilContextTimeout(ctx, pollInterval, duration, true, func(ctx context.Context) (bool, error) {
		pvc, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pvc.Status.Phase == corev1.ClaimBound {
			return false, fmt.Errorf("PVC %s/%s bound to %s, but provisioning should have been refused", namespace, name, pvc.Spec.VolumeName)
		}
		return false, nil
	})
	// Running out of time is the success case here: it means the PVC never bound.
	if wait.Interrupted(err) {
		return nil
	}

	return err
}

// EnsurePodStaysNotRunning is the same idea as EnsurePVCStaysUnbound for a pod that must
// not come up, e.g. because its mount has to fail.
func EnsurePodStaysNotRunning(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, duration time.Duration) error {
	err := wait.PollUntilContextTimeout(ctx, pollInterval, duration, true, func(ctx context.Context) (bool, error) {
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == corev1.PodRunning {
			return false, fmt.Errorf("pod %s/%s reached Running, but its mount should have failed", namespace, name)
		}
		return false, nil
	})
	if wait.Interrupted(err) {
		return nil
	}

	return err
}

// WaitForEventMatching polls the events of one object until one of their messages contains
// substring. Used to show that a failure was reported for the expected reason rather than
// the test merely timing out on something unrelated.
func WaitForEventMatching(ctx context.Context, clientset *kubernetes.Clientset, namespace, objectName, substring string, timeout time.Duration) (string, error) {
	var matched string

	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + objectName,
		})
		if err != nil {
			return false, err
		}
		for _, event := range events.Items {
			if strings.Contains(event.Message, substring) {
				matched = event.Message
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("waiting for an event on %s/%s mentioning %q: %w", namespace, objectName, substring, err)
	}

	return matched, nil
}

// WaitForPodGoneByUID polls until the named pod either disappears or comes back as a
// different object. Needed where something else deletes the pod and a controller recreates
// it under the same name: by the time the test looks, a pod with that name may well exist
// again, so identity has to be compared by UID rather than by name.
func WaitForPodGoneByUID(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, uid types.UID, timeout time.Duration) error {
	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return pod.UID != uid, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for pod %s/%s (uid %s) to be deleted: %w", namespace, name, uid, err)
	}

	return nil
}

// WaitForRunningPodOtherThan polls until a pod matching labelSelector is Running and is not
// the one identified by excludedUID -- the replacement for a pod that was deleted.
func WaitForRunningPodOtherThan(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelSelector string, excludedUID types.UID, timeout time.Duration) (*corev1.Pod, error) {
	var running *corev1.Pod

	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return false, err
		}
		for i, pod := range pods.Items {
			if pod.UID != excludedUID && pod.Status.Phase == corev1.PodRunning {
				running = &pods.Items[i]
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("waiting for a replacement pod matching %q in %s: %w", labelSelector, namespace, err)
	}

	return running, nil
}

// WaitForNoPodsMatching polls until no pod matches labelSelector any more, e.g. after the
// Deployment owning them was deleted.
func WaitForNoPodsMatching(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelSelector string, timeout time.Duration) error {
	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return false, err
		}
		return len(pods.Items) == 0, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for pods matching %q in %s to be deleted: %w", labelSelector, namespace, err)
	}

	return nil
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
