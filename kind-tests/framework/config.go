package framework

import (
	"os"
	"strconv"
	"testing"
)

// Config holds the environment-provided settings needed to run the e2e suite
// against an already-deployed kind cluster + CSI driver + Quobyte client.
type Config struct {
	Kubeconfig         string
	QuobyteAPIURL      string
	QuobyteAPIUser     string
	QuobyteAPIPassword string
	CSIProvisionerName string
	// Created by the script during the test run
	Namespace string
	// Created by the script during the test run - only override in env if needed
	// some specific tenant
	QuobyteTenant string
	// StorageClassName optionally overrides the StorageClass name the test
	// generates for itself. Not required -- if unset, the test falls back to
	// its own generated name.
	StorageClassName string
	// ArtifactsDir optionally names a directory the test should write copies
	// of the Secret/StorageClass it creates to, before deleting them. Not
	// required -- if unset, artifact dumping is skipped.
	ArtifactsDir string
	// EnableAccessKeyMounts mirrors the ENABLE_ACCESS_KEY_MOUNTS setting of the
	// "env" file the driver/client were deployed with: when true, mounts use
	// Quobyte access keys, so the Secret a test creates must carry
	// accessKeyId/accessKeySecret instead of user/password (see
	// src/driver/node.go).
	EnableAccessKeyMounts bool
	// EnableSnapshots mirrors the ENABLE_SNAPSHOTS setting of the "env" file and
	// is passed on to the upstream Kubernetes e2e suite (see RunUpstreamE2E).
	EnableSnapshots bool
	// UpstreamE2EScript is the path to kind-tests/e2e, the script that runs the
	// upstream sig-storage "external storage" suite. Set by run_test; only the
	// tests that drive that suite need it (see RunUpstreamE2E).
	UpstreamE2EScript string
	// RepoRoot is the quobyte-csi-driver checkout UpstreamE2EScript must run
	// from. Set by run_test alongside UpstreamE2EScript.
	RepoRoot string
}

// LoadConfig reads the required environment variables and fails the test
// immediately if any of them are missing.
func LoadConfig(t *testing.T) Config {
	t.Helper()

	cfg := Config{
		Kubeconfig:         os.Getenv("KUBECONFIG"),
		Namespace:          os.Getenv("NAMESPACE"),
		QuobyteAPIURL:      os.Getenv("QUOBYTE_API_URL"),
		QuobyteAPIUser:     os.Getenv("QUOBYTE_API_USER"),
		QuobyteAPIPassword: os.Getenv("QUOBYTE_API_PASSWORD"),
		QuobyteTenant:      os.Getenv("QUOBYTE_TENANT"),
		CSIProvisionerName: os.Getenv("CSI_PROVISIONER_NAME"),
		StorageClassName:   os.Getenv("STORAGE_CLASS_NAME"),
		ArtifactsDir:       os.Getenv("ARTIFACTS_DIR"),

		EnableAccessKeyMounts: boolEnv("ENABLE_ACCESS_KEY_MOUNTS"),
		EnableSnapshots:       boolEnv("ENABLE_SNAPSHOTS"),
		UpstreamE2EScript:     os.Getenv("UPSTREAM_E2E_SCRIPT"),
		RepoRoot:              os.Getenv("REPO_ROOT"),
	}

	required := map[string]string{
		"KUBECONFIG":           cfg.Kubeconfig,
		"NAMESPACE":            cfg.Namespace,
		"QUOBYTE_API_URL":      cfg.QuobyteAPIURL,
		"QUOBYTE_API_USER":     cfg.QuobyteAPIUser,
		"QUOBYTE_API_PASSWORD": cfg.QuobyteAPIPassword,
		"QUOBYTE_TENANT":       cfg.QuobyteTenant,
		"CSI_PROVISIONER_NAME": cfg.CSIProvisionerName,
	}
	for name, value := range required {
		if value == "" {
			t.Fatalf("required environment variable %s is not set", name)
		}
	}

	return cfg
}

// boolEnv reads an optional boolean environment variable, treating anything
// that isn't parseable as false. The values come from the shell "env" files
// run_test sources, so they're written as true/false.
func boolEnv(name string) bool {
	value, err := strconv.ParseBool(os.Getenv(name))
	if err != nil {
		return false
	}
	return value
}
