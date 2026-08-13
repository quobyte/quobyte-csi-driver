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
	// UseK8SNamespaceAsTenant mirrors the USE_K8S_NAMESPACE_AS_TENANT setting of the
	// "env" file, which that file also has to pass on to the driver as
	// quobyte.useK8SNamespaceAsTenant (via CSI_HELM_SET) -- the same split as
	// EnableSnapshots. With it, a StorageClass that names no quobyteTenant is
	// provisioned into the tenant named after the PVC's namespace, instead of leaving
	// the tenant to the Secret's credentials (see CreateVolume in
	// src/driver/controller.go).
	UseK8SNamespaceAsTenant bool
	// UseSeparateMountSecret mirrors USE_SEPARATE_MOUNT_SECRET: split the driver's two
	// uses of a Secret across two of them, the way
	// kind-tests/test-configs/local_cluster_accesskeys_2 did on master -- a management
	// access key for provisioning/expansion, and a separate data access key that the
	// node-publish secret carries for mounting. Only meaningful together with
	// EnableAccessKeyMounts.
	UseSeparateMountSecret bool
	// EnableVolumeMetrics mirrors ENABLE_VOLUME_METRICS, which the "env" file also has to
	// pass on to the driver as quobyte.enableVolumeMetrics (via CSI_HELM_SET) -- the same
	// split as EnableSnapshots. With it off, NodeGetVolumeStats refuses every request and
	// the kubelet exports no kubelet_volume_stats_* series for the claim (see
	// src/driver/node.go).
	//
	// Defaults to true, matching the chart.
	EnableVolumeMetrics bool
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
	// SharedVolumeOptions mirrors the shared volume settings of the "env" file.
	SharedVolumeOptions SharedVolumeOptions
}

// SharedVolumeOptions describes whether the test provisions through a Quobyte shared
// volume: one volume that every PVC becomes a subdirectory of, instead of a volume per
// PVC. The driver switches to that mode purely on the StorageClass carrying a
// "sharedVolumeName" parameter -- see src/driver/controller.go.
type SharedVolumeOptions struct {
	// EnableSharedVolume mirrors USE_SHARED_VOLUME: put "sharedVolumeName" into the
	// StorageClass the test builds.
	EnableSharedVolume bool
	// Only used if the the EnableSharedVolume is set to true. Mirrors
	// PRE_CREATE_SHARED_VOLUME: create that volume through the Quobyte API during
	// test setup, rather than leaving the driver to create it on the first
	// provisioning request.
	PreCreateSharedVolume bool
	// Name optionally pins the shared volume's name via SHARED_VOLUME_NAME. Not
	// required -- if unset, the test generates a name unique to the run, the same
	// way it names its StorageClass.
	Name string
	// UseDeleteFilesTask mirrors USE_DELETE_FILES_TASK, which the "env" file also has to
	// pass on to the driver as quobyte.useDeleteFilesTaskForSharedVolumeCleanup (via
	// CSI_HELM_SET) -- the same split as EnableSnapshots. It decides which branch of
	// DeleteVolume a shared volume PVC takes: the DELETE_FILES_IN_VOLUMES task, or
	// renaming the subdirectory to a delete marker for the driver's own sweep (see
	// src/driver/controller.go and src/driver/shared_volume_directory_deleter.go).
	//
	// Defaults to true, matching the chart, so only an "env" file that turns it off has
	// to mention it at all.
	UseDeleteFilesTask bool
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

		EnableAccessKeyMounts:   boolEnv("ENABLE_ACCESS_KEY_MOUNTS"),
		UseK8SNamespaceAsTenant: boolEnv("USE_K8S_NAMESPACE_AS_TENANT"),
		UseSeparateMountSecret:  boolEnv("USE_SEPARATE_MOUNT_SECRET"),

		EnableVolumeMetrics: boolEnvWithDefault("ENABLE_VOLUME_METRICS", true),

		EnableSnapshots:   boolEnv("ENABLE_SNAPSHOTS"),
		UpstreamE2EScript: os.Getenv("UPSTREAM_E2E_SCRIPT"),
		RepoRoot:          os.Getenv("REPO_ROOT"),
		SharedVolumeOptions: SharedVolumeOptions{
			EnableSharedVolume:    boolEnv("USE_SHARED_VOLUME"),
			PreCreateSharedVolume: boolEnv("PRE_CREATE_SHARED_VOLUME"),
			Name:                  os.Getenv("SHARED_VOLUME_NAME"),
			UseDeleteFilesTask:    boolEnvWithDefault("USE_DELETE_FILES_TASK", true),
		},
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

	// Pre-creating a shared volume nothing then provisions through is never what an
	// "env" file meant to say, so say so rather than silently ignoring it.
	if cfg.SharedVolumeOptions.PreCreateSharedVolume && !cfg.SharedVolumeOptions.EnableSharedVolume {
		t.Fatal("PRE_CREATE_SHARED_VOLUME is set without USE_SHARED_VOLUME: the pre-created volume would not be used")
	}

	// A separate mount secret exists to carry file system access keys. Without access
	// key mounts the node plugin wants the user/password the API secret already has, so
	// splitting them would test nothing.
	if cfg.UseSeparateMountSecret && !cfg.EnableAccessKeyMounts {
		t.Fatal("USE_SEPARATE_MOUNT_SECRET is set without ENABLE_ACCESS_KEY_MOUNTS: a separate mount secret only makes sense for access key mounts")
	}

	return cfg
}

// boolEnv reads an optional boolean environment variable, treating anything
// that isn't parseable as false. The values come from the shell "env" files
// run_test sources, so they're written as true/false.
func boolEnv(name string) bool {
	return boolEnvWithDefault(name, false)
}

// boolEnvWithDefault is boolEnv for the settings whose chart default is true: an "env" file
// that says nothing about them means the chart default, not false.
func boolEnvWithDefault(name string, fallback bool) bool {
	value, err := strconv.ParseBool(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return value
}
