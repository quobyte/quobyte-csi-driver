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
	Name        string
	Provisioner string
	Tenant      string
	// SecretName/SecretNamespace hold the Quobyte management API credentials, used to
	// provision and to expand volumes.
	SecretName      string
	SecretNamespace string
	// MountSecretName/MountSecretNamespace optionally point the node-publish secret at
	// a second Secret holding file system access credentials, splitting API access from
	// data access across two Secrets (as in the local_cluster_accesskeys_2 example).
	// Empty means the API Secret above is used for mounting too.
	MountSecretName      string
	MountSecretNamespace string
	// SharedVolumeName, when set, adds the sharedVolumeName parameter, so every PVC
	// against this StorageClass is provisioned as a subdirectory of that one Quobyte
	// volume. Empty means a volume per PVC.
	SharedVolumeName string
	// ExtraParameters are put into the StorageClass after everything above, so a test can
	// reach the parameters CreateVolume accepts but nothing here spells out -- user, group,
	// labels, accessMode (see src/driver/controller.go) -- and can override the defaults
	// below, createQuota in particular.
	ExtraParameters map[string]string
}

// NewStorageClass builds a StorageClass object referencing the given secret
// for provisioning/expansion/mounting, expressed as a typed object so the
// test can generate a fresh, uniquely-named StorageClass per run instead of
// depending on one being pre-applied to the cluster.
func NewStorageClass(opts StorageClassOptions) *storagev1.StorageClass {
	allowVolumeExpansion := true
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete

	// Mounting falls back to the API secret unless a separate one was given.
	mountSecretName, mountSecretNamespace := opts.MountSecretName, opts.MountSecretNamespace
	if mountSecretName == "" {
		mountSecretName, mountSecretNamespace = opts.SecretName, opts.SecretNamespace
	}

	parameters := map[string]string{
		// Ignored by the driver for shared volumes, where the quota would apply to a
		// subdirectory and has to be set by an admin at tenant level instead.
		"createQuota": "true",

		"csi.storage.k8s.io/provisioner-secret-name":            opts.SecretName,
		"csi.storage.k8s.io/provisioner-secret-namespace":       opts.SecretNamespace,
		"csi.storage.k8s.io/controller-expand-secret-name":      opts.SecretName,
		"csi.storage.k8s.io/controller-expand-secret-namespace": opts.SecretNamespace,
		"csi.storage.k8s.io/node-publish-secret-name":           mountSecretName,
		"csi.storage.k8s.io/node-publish-secret-namespace":      mountSecretNamespace,
	}
	// Left out entirely when empty rather than set to "": a StorageClass without
	// quobyteTenant is what makes the driver fall back to the PVC's namespace, when it
	// runs with useK8SNamespaceAsTenant (see CreateVolume in src/driver/controller.go).
	if opts.Tenant != "" {
		parameters["quobyteTenant"] = opts.Tenant
	}
	if opts.SharedVolumeName != "" {
		parameters[SharedVolumeNameParameter] = opts.SharedVolumeName
	}
	// Last, so a test can override what is set above rather than only add to it.
	for name, value := range opts.ExtraParameters {
		parameters[name] = value
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
