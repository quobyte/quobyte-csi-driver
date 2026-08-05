package framework

import (
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewSecret builds the Quobyte API credentials Secret referenced by a
// StorageClass's provisioner/controller-expand/node-publish secret
// parameters.
func NewSecret(name, namespace, user, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretType("kubernetes.io/quobyte"),
		Data: map[string][]byte{
			"user":     []byte(user),
			"password": []byte(password),
		},
	}
}

// NewPVC builds a PersistentVolumeClaim object mirroring
// kind-tests/quobyte-k8s-resources/usage-examples/01_getting_started/04-testpvc.yaml,
// parameterized by name/namespace/storageClass/size.
func NewPVC(name, namespace, storageClass, size string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resourceapi.MustParse(size),
				},
			},
			StorageClassName: &storageClass,
		},
	}
}

// NewPod builds a Pod object mounting the given PVC at mountPath, mirroring
// kind-tests/quobyte-k8s-resources/usage-examples/01_getting_started/05_testpod.yaml
// but using busybox with a long-running command so the e2e suite can exec
// into it to write/read files.
func NewPod(name, namespace, pvcName, mountPath string) *corev1.Pod {
	const volumeName = "test-storage"

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": name},
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "test-container",
					Image:   "busybox",
					Command: []string{"sh", "-c", "sleep 3600"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      volumeName,
							MountPath: mountPath,
						},
					},
				},
			},
		},
	}
}
