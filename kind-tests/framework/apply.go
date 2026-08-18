package framework

import (
	"context"
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Creating a resource and arranging for its removal is the same three steps everywhere --
// create, fail the test if that did not work, register a cleanup that deletes it and waits
// for it to actually be gone. The Apply* helpers below do exactly that, so a test reads as
// what it sets up rather than as bookkeeping.
//
// Call them in dependency order (secret -> storage class -> pvc -> pod): cleanups run LIFO,
// so that order tears down bottom-up, and each step waits for its resource to disappear
// before the next one starts (deleting a k8s object only marks it for deletion). A wait
// that times out is logged and the remaining steps still run -- see CleanupUnlessFailed,
// which skips all of this when the test failed so the cluster can be inspected.
const (
	secretDeleteTimeout     = time.Minute
	storageClassTimeout     = time.Minute
	pvcDeleteTimeout        = 2 * time.Minute
	pvDeleteTimeout         = 3 * time.Minute
	podDeleteTimeout        = 2 * time.Minute
	deploymentDeleteTimeout = 2 * time.Minute
)

// ApplySecret creates a Secret and registers its removal.
func ApplySecret(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, secret *corev1.Secret) {
	t.Helper()

	if _, err := clientset.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating secret %s/%s: %v", secret.Namespace, secret.Name, err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("secret %s/%s", secret.Namespace, secret.Name), func() {
		cleanupCtx := context.Background()
		_ = clientset.CoreV1().Secrets(secret.Namespace).Delete(cleanupCtx, secret.Name, metav1.DeleteOptions{})
		if err := WaitForSecretDeleted(cleanupCtx, clientset, secret.Namespace, secret.Name, secretDeleteTimeout); err != nil {
			t.Logf("warning: %v", err)
		}
	})
}

// ApplyStorageClass creates a StorageClass and registers its removal.
func ApplyStorageClass(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, storageClass *storagev1.StorageClass) {
	t.Helper()

	if _, err := clientset.StorageV1().StorageClasses().Create(ctx, storageClass, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating storage class %s: %v", storageClass.Name, err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("storage class %s", storageClass.Name), func() {
		cleanupCtx := context.Background()
		_ = clientset.StorageV1().StorageClasses().Delete(cleanupCtx, storageClass.Name, metav1.DeleteOptions{})
		if err := WaitForStorageClassDeleted(cleanupCtx, clientset, storageClass.Name, storageClassTimeout); err != nil {
			t.Logf("warning: %v", err)
		}
	})
}

// ApplyPV creates a PersistentVolume and registers its removal.
func ApplyPV(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, pv *corev1.PersistentVolume) {
	t.Helper()

	if _, err := clientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating pv %s: %v", pv.Name, err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("pv %s", pv.Name), func() {
		cleanupCtx := context.Background()
		_ = clientset.CoreV1().PersistentVolumes().Delete(cleanupCtx, pv.Name, metav1.DeleteOptions{})
		if err := WaitForPVDeleted(cleanupCtx, clientset, pv.Name, pvDeleteTimeout); err != nil {
			t.Logf("warning: %v", err)
		}
	})
}

// ApplyPVC creates a PersistentVolumeClaim and registers its removal. It does not wait for
// the claim to bind -- a test that expects it to must say so with WaitForPVCBound, and one
// that expects it not to with EnsurePVCStaysUnbound.
//
// Cleanup waits for the claim to go, and then for its PV as well if that PV is reclaimed by
// deletion: that second wait is what shows the driver finished deleting the Quobyte volume,
// which has to happen while the credentials to do it are still around.
func ApplyPVC(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, pvc *corev1.PersistentVolumeClaim) {
	t.Helper()

	if _, err := clientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating pvc %s/%s: %v", pvc.Namespace, pvc.Name, err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("pvc %s/%s", pvc.Namespace, pvc.Name), func() {
		cleanupCtx := context.Background()

		// Read the claim before deleting it, to learn which PV to wait for.
		var boundPVName string
		if bound, err := clientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(cleanupCtx, pvc.Name, metav1.GetOptions{}); err == nil {
			boundPVName = bound.Spec.VolumeName
		}

		_ = clientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(cleanupCtx, pvc.Name, metav1.DeleteOptions{})
		if err := WaitForPVCDeleted(cleanupCtx, clientset, pvc.Namespace, pvc.Name, pvcDeleteTimeout); err != nil {
			t.Logf("warning: %v", err)
			return
		}
		if boundPVName == "" || !pvIsReclaimedByDeletion(cleanupCtx, clientset, boundPVName) {
			return
		}
		if err := WaitForPVDeleted(cleanupCtx, clientset, boundPVName, pvDeleteTimeout); err != nil {
			t.Logf("warning: %v", err)
		}
	})
}

// ApplyPod creates a Pod and registers its removal. It does not wait for the pod to run.
func ApplyPod(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, pod *corev1.Pod) {
	t.Helper()

	if _, err := clientset.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("pod %s/%s", pod.Namespace, pod.Name), func() {
		cleanupCtx := context.Background()
		_ = clientset.CoreV1().Pods(pod.Namespace).Delete(cleanupCtx, pod.Name, metav1.DeleteOptions{})
		// Gone, not merely terminating, before the next step takes away the volume it
		// still has mounted.
		if err := WaitForPodDeleted(cleanupCtx, clientset, pod.Namespace, pod.Name, podDeleteTimeout); err != nil {
			t.Logf("warning: %v", err)
		}
	})
}

// ApplyDeployment creates a Deployment and registers its removal, waiting for its pods to
// go with it -- they are what hold the volume mounted.
func ApplyDeployment(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, deployment *appsv1.Deployment) {
	t.Helper()

	if _, err := clientset.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating deployment %s/%s: %v", deployment.Namespace, deployment.Name, err)
	}
	CleanupUnlessFailed(t, fmt.Sprintf("deployment %s/%s", deployment.Namespace, deployment.Name), func() {
		cleanupCtx := context.Background()
		_ = clientset.AppsV1().Deployments(deployment.Namespace).Delete(cleanupCtx, deployment.Name, metav1.DeleteOptions{})
		if err := WaitForNoPodsMatching(cleanupCtx, clientset, deployment.Namespace,
			metav1.FormatLabelSelector(deployment.Spec.Selector), deploymentDeleteTimeout); err != nil {
			t.Logf("warning: %v", err)
		}
	})
}

// pvIsReclaimedByDeletion reports whether deleting a claim on this PV is supposed to take
// the PV (and the Quobyte volume behind it) with it. A PV that is missing or Retained is
// not, so there is nothing to wait for.
func pvIsReclaimedByDeletion(ctx context.Context, clientset *kubernetes.Clientset, name string) bool {
	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false
	}

	return pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete
}
