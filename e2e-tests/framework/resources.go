package framework

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestContainerName is the container NewPod puts the Quobyte mount in, and the one
// WriteFileInPod/ReadFileInPod exec into.
const TestContainerName = "test-container"

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

// NewAccessKeySecret builds the same Secret as NewSecret, but holding Quobyte
// access key credentials instead of a user/password pair. Required instead of
// NewSecret when the driver runs with access key mounts enabled -- the node
// plugin refuses to mount without accessKeyId/accessKeySecret in the mount
// secret (see src/driver/node.go).
func NewAccessKeySecret(name, namespace, accessKeyID, accessKeySecret string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretType("kubernetes.io/quobyte"),
		Data: map[string][]byte{
			"accessKeyId":     []byte(accessKeyID),
			"accessKeySecret": []byte(accessKeySecret),
		},
	}
}

// NewPVC builds a PersistentVolumeClaim object mirroring
// e2e-tests/quobyte-k8s-resources/usage-examples/01_getting_started/04-testpvc.yaml,
// parameterized by name/namespace/storageClass/size, with the ReadWriteOnce access mode
// almost every test wants. Use NewPVCWithAccessModes for anything else.
func NewPVC(name, namespace, storageClass, size string) *corev1.PersistentVolumeClaim {
	return NewPVCWithAccessModes(name, namespace, storageClass, size, corev1.ReadWriteOnce)
}

// NewPVCWithAccessModes is NewPVC with the access modes spelled out, for the tests that
// mount one volume from several nodes at once (ReadWriteMany) -- which is what a Quobyte
// volume actually supports, and what the driver declares to the upstream suite.
func NewPVCWithAccessModes(name, namespace, storageClass, size string, accessModes ...corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resourceapi.MustParse(size),
				},
			},
			StorageClassName: &storageClass,
		},
	}
}

// PVOptions describes a PersistentVolume pointed at a Quobyte volume that already exists,
// as opposed to one the provisioner created.
type PVOptions struct {
	Name   string
	Driver string
	// VolumeHandle is "<tenant>|<volume>", or "<tenant>|<volume>|<subDirectory>" to mount
	// a subdirectory of that volume rather than its root -- see processVolumeHandle and
	// formMountPath in src/driver/node.go. Tenant and volume may each be a name or a UUID;
	// names are only resolvable if the secret below carries API credentials.
	VolumeHandle string
	Size         string
	// SecretName/SecretNamespace become the PV's nodePublishSecretRef. A statically
	// provisioned volume has no StorageClass to take mount credentials from, so without
	// this the node plugin gets no secrets at all.
	SecretName      string
	SecretNamespace string
	// AccessModes defaults to ReadWriteOnce when empty, matching NewPVC.
	AccessModes []corev1.PersistentVolumeAccessMode
}

// NewPreProvisionedPV builds a PersistentVolume bound to an existing Quobyte volume. Its
// reclaim policy is Retain: the volume belongs to whoever created it, and deleting the PV
// must not take it away.
func NewPreProvisionedPV(opts PVOptions) *corev1.PersistentVolume {
	accessModes := opts.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: opts.Name,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: accessModes,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resourceapi.MustParse(opts.Size),
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			// Empty on purpose: the PVC binds to this PV by name, and naming a
			// StorageClass would send the claim through the provisioner instead.
			StorageClassName: "",
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       opts.Driver,
					VolumeHandle: opts.VolumeHandle,
					NodePublishSecretRef: &corev1.SecretReference{
						Name:      opts.SecretName,
						Namespace: opts.SecretNamespace,
					},
				},
			},
		},
	}
}

// NewPVCForPV builds a PersistentVolumeClaim that binds to one specific pre-provisioned
// PV by name instead of asking a StorageClass for a new volume.
func NewPVCForPV(name, namespace, pvName, size string) *corev1.PersistentVolumeClaim {
	// Must be set and empty: left nil, the cluster's default StorageClass would apply.
	const noStorageClass = ""

	pvc := NewPVC(name, namespace, noStorageClass, size)
	pvc.Spec.VolumeName = pvName

	return pvc
}

// NewDeployment builds a single-replica Deployment of the pod NewPod describes. Used where
// a test needs the pod to come back after something deletes it -- a bare pod would simply
// stay gone.
func NewDeployment(name, namespace, pvcName, mountPath string) *appsv1.Deployment {
	replicas := int32(1)
	pod := NewPod(name, namespace, pvcName, mountPath)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: pod.Labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: pod.Labels},
				Spec:       pod.Spec,
			},
		},
	}
}

// NewPod builds a Pod object mounting the given PVC at mountPath, mirroring
// e2e-tests/quobyte-k8s-resources/usage-examples/01_getting_started/05_testpod.yaml
// but using busybox with a long-running command so the e2e suite can exec
// into it to write/read files.
// NewPodOnNode is NewPod pinned to one named node. A test that has to show a volume is
// mounted from two nodes at once has to place its pods itself: left to the scheduler, two
// pods of the same shape may well land on the same node and the test would pass without
// ever crossing a node boundary.
func NewPodOnNode(name, namespace, pvcName, mountPath, nodeName string) *corev1.Pod {
	pod := NewPod(name, namespace, pvcName, mountPath)
	pod.Spec.NodeName = nodeName

	return pod
}

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
					Name:    TestContainerName,
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
