package sharedvolume

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
)

// The mode the driver gives a claim's subdirectory when the StorageClass does not say
// otherwise: DefaultPVCAccessModeInsideSharedVolume is 1700, but the sticky bit never
// reaches the directory -- createDirectory (src/driver/shared_volume_mapper.go) goes through
// os.Mkdir, which keeps only the user/group/other bits, as the comment on that constant in
// src/driver/controller.go says. 700 is therefore the mode to expect, and a test asserting
// 1700 would be asserting the intent rather than the behaviour.
const defaultSubDirectoryMode = "700"

// The mode the parameterised case asks for. Chosen so that no plausible umask in the driver
// container can change it -- os.Mkdir applies one, so a mode with group or other write bits
// (770, say) would arrive as something else and the failure would say nothing about the
// driver.
const requestedSubDirectoryMode = "755"

// TestSharedVolumeAccessModeAppliesToSubdirectory covers the shared volume half of the
// accessMode StorageClass parameter; the volume-per-claim half is in
// dynamic_provisioning/storage_class_parameters_test.go.
//
// The two sides behave differently, which is what makes this worth its own test. Without a
// shared volume the parameter becomes the volume's own access mode. With one it is ignored
// at volume level -- the shared volume keeps the 1777 it needs to hold a subdirectory per
// claim -- and is applied to the claim's subdirectory instead (see the isSharedVolume
// branches of CreateVolume in src/driver/controller.go). Nothing asserted either side of
// that before, so a change that applied the parameter to the shared volume itself would have
// gone unnoticed until it broke a customer's other claims.
func TestSharedVolumeAccessModeAppliesToSubdirectory(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	require.True(t, cfg.SharedVolumeOptions.EnableSharedVolume,
		"this test only makes sense with USE_SHARED_VOLUME set; check this directory's env files")

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-svmode-secret-%d", suffix)

	sharedVolumeName := cfg.SharedVolumeOptions.Name
	if sharedVolumeName == "" {
		sharedVolumeName = fmt.Sprintf("e2e-svmode-vol-%d", suffix)
	}

	if cfg.SharedVolumeOptions.PreCreateSharedVolume {
		sharedVolumeUUID, err := framework.CreateSharedVolume(quobyteClient, sharedVolumeName, tenantID)
		require.NoError(t, err, "pre-creating shared volume %q", sharedVolumeName)
		framework.CleanupUnlessFailed(t, fmt.Sprintf("shared volume %s", sharedVolumeName), func() {
			if err := framework.DeleteVolume(quobyteClient, sharedVolumeUUID); err != nil {
				t.Logf("warning: %v", err)
			}
		})
	}

	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID))

	mount := func(caseName string, params map[string]string) string {
		names := struct{ sc, pvc, pod string }{
			sc:  fmt.Sprintf("e2e-svmode-sc-%s-%d", caseName, suffix),
			pvc: fmt.Sprintf("e2e-svmode-pvc-%s-%d", caseName, suffix),
			pod: fmt.Sprintf("e2e-svmode-pod-%s-%d", caseName, suffix),
		}

		framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
			Name:             names.sc,
			Provisioner:      cfg.CSIProvisionerName,
			Tenant:           cfg.QuobyteTenant,
			SecretName:       secretName,
			SecretNamespace:  cfg.Namespace,
			SharedVolumeName: sharedVolumeName,
			ExtraParameters:  params,
		}))

		framework.ApplyPVC(t, ctx, clientset, framework.NewPVC(names.pvc, cfg.Namespace, names.sc, "1Gi"))
		_, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, names.pvc, 3*time.Minute)
		require.NoErrorf(t, err, "waiting for the claim of the %q case to bind", caseName)

		framework.ApplyPod(t, ctx, clientset, framework.NewPod(names.pod, cfg.Namespace, names.pvc, mountPath))
		_, err = framework.WaitForPodRunning(ctx, clientset, cfg.Namespace, names.pod, 2*time.Minute)
		require.NoErrorf(t, err, "waiting for the pod of the %q case to run", caseName)

		stdout, stderr, err := framework.ExecInPod(ctx, restConfig, clientset, cfg.Namespace, names.pod,
			framework.TestContainerName, []string{"stat", "-c", "%a", mountPath})
		require.NoErrorf(t, err, "reading the mode of the %q case's mount: stderr=%s", caseName, stderr)

		return strings.TrimSpace(stdout)
	}

	require.Equal(t, defaultSubDirectoryMode, mount("default", nil),
		"a claim with no accessMode did not get the driver's default mode for a subdirectory of a shared volume")

	require.Equal(t, requestedSubDirectoryMode, mount("configured", map[string]string{
		"accessMode": requestedSubDirectoryMode,
	}), "accessMode did not reach the claim's subdirectory of the shared volume")
}
