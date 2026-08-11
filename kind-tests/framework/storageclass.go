package framework

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SharedVolumeNameParameter is the StorageClass parameter that switches the driver from
// a volume per PVC to subdirectories of one shared volume (SharedVolumeNameKey in
// src/driver/controller.go).
const SharedVolumeNameParameter = "sharedVolumeName"

// StorageClassOptions describes the StorageClass a test builds for itself.
type StorageClassOptions struct {
	Name            string
	Provisioner     string
	Tenant          string
	SecretName      string
	SecretNamespace string
	// SharedVolumeName, when set, adds the sharedVolumeName parameter, so every PVC
	// against this StorageClass is provisioned as a subdirectory of that one Quobyte
	// volume. Empty means a volume per PVC.
	SharedVolumeName string
}

// NewStorageClass builds a StorageClass object referencing the given secret
// for provisioning/expansion/mounting, expressed as a typed object so the
// test can generate a fresh, uniquely-named StorageClass per run instead of
// depending on one being pre-applied to the cluster.
func NewStorageClass(opts StorageClassOptions) *storagev1.StorageClass {
	allowVolumeExpansion := true
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete

	parameters := map[string]string{
		"quobyteTenant": opts.Tenant,
		// Ignored by the driver for shared volumes, where the quota would apply to a
		// subdirectory and has to be set by an admin at tenant level instead.
		"createQuota": "true",

		"csi.storage.k8s.io/provisioner-secret-name":            opts.SecretName,
		"csi.storage.k8s.io/provisioner-secret-namespace":       opts.SecretNamespace,
		"csi.storage.k8s.io/controller-expand-secret-name":      opts.SecretName,
		"csi.storage.k8s.io/controller-expand-secret-namespace": opts.SecretNamespace,
		"csi.storage.k8s.io/node-publish-secret-name":           opts.SecretName,
		"csi.storage.k8s.io/node-publish-secret-namespace":      opts.SecretNamespace,
	}
	if opts.SharedVolumeName != "" {
		parameters[SharedVolumeNameParameter] = opts.SharedVolumeName
	}

	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: opts.Name,
		},
		Provisioner:          opts.Provisioner,
		AllowVolumeExpansion: &allowVolumeExpansion,
		ReclaimPolicy:        &reclaimPolicy,
		Parameters:           parameters,
	}
}
