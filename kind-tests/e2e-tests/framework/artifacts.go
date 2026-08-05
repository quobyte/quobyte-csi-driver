package framework

import (
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// DumpSecretYAML writes secret to <dir>/<prefix>-secret.yaml as a standalone,
// kubectl-apply-able manifest, so it can be recreated later even after the
// test's own cleanup has deleted it from the cluster. A no-op (returns "",
// nil) when dir is empty.
func DumpSecretYAML(dir, prefix string, secret *corev1.Secret) (string, error) {
	dump := secret.DeepCopy()
	dump.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}
	return writeResourceYAML(dir, prefix+"-secret.yaml", dump)
}

// DumpStorageClassYAML writes sc to <dir>/<prefix>-storageclass.yaml as a
// standalone, kubectl-apply-able manifest, so it can be recreated later even
// after the test's own cleanup has deleted it from the cluster. A no-op
// (returns "", nil) when dir is empty.
func DumpStorageClassYAML(dir, prefix string, sc *storagev1.StorageClass) (string, error) {
	dump := sc.DeepCopy()
	dump.TypeMeta = metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClass"}
	return writeResourceYAML(dir, prefix+"-storageclass.yaml", dump)
}

func writeResourceYAML(dir, filename string, obj interface{}) (string, error) {
	if dir == "" {
		return "", nil
	}

	data, err := yaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("marshaling %s to yaml: %w", filename, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating artifacts dir %s: %w", dir, err)
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}

	return path, nil
}
