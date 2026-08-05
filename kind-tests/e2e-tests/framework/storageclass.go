package framework

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewStorageClass builds a StorageClass object referencing the given secret
// for provisioning/expansion/mounting, mirroring
// kind-tests/test-configs/local_cluster/k8s_storage_class.yaml but expressed
// as a typed object so the test can generate a fresh, uniquely-named
// StorageClass per run instead of depending on one being pre-applied to the
// cluster.
func NewStorageClass(name, provisioner, tenant, secretName, secretNamespace string) *storagev1.StorageClass {
	allowVolumeExpansion := true
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete

	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:          provisioner,
		AllowVolumeExpansion: &allowVolumeExpansion,
		ReclaimPolicy:        &reclaimPolicy,
		Parameters: map[string]string{
			"quobyteTenant": tenant,
			"createQuota":   "true",

			"csi.storage.k8s.io/provisioner-secret-name":            secretName,
			"csi.storage.k8s.io/provisioner-secret-namespace":       secretNamespace,
			"csi.storage.k8s.io/controller-expand-secret-name":      secretName,
			"csi.storage.k8s.io/controller-expand-secret-namespace": secretNamespace,
			"csi.storage.k8s.io/node-publish-secret-name":           secretName,
			"csi.storage.k8s.io/node-publish-secret-namespace":      secretNamespace,
		},
	}
}
