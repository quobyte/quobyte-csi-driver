package framework

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// UpstreamE2EOptions selects what the upstream suite is run against.
type UpstreamE2EOptions struct {
	// StorageClassFile is a standalone StorageClass manifest, as written by
	// DumpStorageClassYAML. kind-tests/e2e hands it to e2e.test as
	// "StorageClass: FromFile", which creates its own copy of it per test, so
	// the StorageClass does not have to exist in the cluster -- but everything
	// it references (in particular the Secret named in its parameters) does.
	StorageClassFile string
	// SnapshotClassFile is the equivalent manifest for snapshot tests. Only
	// meaningful when the driver was deployed with snapshots enabled; may be
	// empty otherwise.
	SnapshotClassFile string
}

// RunUpstreamE2E runs the upstream Kubernetes sig-storage "external storage"
// suite against the already-deployed quobyte-csi-driver, by invoking the
// kind-tests/e2e script (cfg.UpstreamE2EScript) the same way run_test would.
// Ginkgo's output is streamed to the test's stdout/stderr rather than buffered,
// so a long suite reports progress as it goes.
func RunUpstreamE2E(ctx context.Context, cfg Config, opts UpstreamE2EOptions) error {
	if cfg.UpstreamE2EScript == "" {
		return fmt.Errorf("UPSTREAM_E2E_SCRIPT is not set; cannot run the upstream Kubernetes e2e suite")
	}
	if cfg.RepoRoot == "" {
		return fmt.Errorf("REPO_ROOT is not set; the upstream e2e script must run from the repository root")
	}
	if opts.StorageClassFile == "" {
		return fmt.Errorf("no StorageClass manifest given to run the upstream Kubernetes e2e suite against")
	}

	cmd := exec.CommandContext(ctx, "bash", cfg.UpstreamE2EScript)
	cmd.Dir = cfg.RepoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"KUBECONFIG="+cfg.Kubeconfig,
		"CSI_PROVISIONER_NAME="+cfg.CSIProvisionerName,
		"STORAGE_CLASS="+opts.StorageClassFile,
		"SNAPSHOT_CLASS="+opts.SnapshotClassFile,
		"ENABLE_SNAPSHOTS="+strconv.FormatBool(cfg.EnableSnapshots),
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running upstream Kubernetes e2e suite (%s): %w", cfg.UpstreamE2EScript, err)
	}

	return nil
}
