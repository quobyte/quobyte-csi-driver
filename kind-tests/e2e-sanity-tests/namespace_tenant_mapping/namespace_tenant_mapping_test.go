package namespacetenantmapping

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TestNamespaceIsUsedAsTenant covers the "Namespace-to-tenant mapping tests (tenant
// overrides)" entry of the TODO list in kind-tests/e2e-sanity-tests/README.md.
//
// Deployed with quobyte.useK8SNamespaceAsTenant=true, the driver takes the tenant from the
// PVC's Kubernetes namespace -- but only when the StorageClass leaves quobyteTenant unset.
// A StorageClass that names a tenant still wins, and that override is the second half of
// this scenario. See CreateVolume in src/driver/controller.go.
//
// Both halves run against the same driver deployment, so the two claims differ in nothing
// but their StorageClass, which is what makes the comparison meaningful.
func TestNamespaceIsUsedAsTenant(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, _, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-nstenant-secret-%d", suffix)

	// The mapping is namespace name -> tenant name and the driver does not create tenants,
	// so the tenant named after this run's namespace has to exist first. It is this test's
	// own (the namespace is generated per run by run_test), so it goes again afterwards.
	namespaceTenantID, err := framework.EnsureTenant(quobyteClient, cfg.Namespace, cfg.QuobyteAPIUser)
	require.NoError(t, err, "creating the tenant named after namespace %q", cfg.Namespace)
	t.Logf("tenant %s (%s) stands in for namespace %s", cfg.Namespace, namespaceTenantID, cfg.Namespace)
	framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte tenant %s", cfg.Namespace), func() {
		if err := framework.DeleteTenant(quobyteClient, cfg.Namespace); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	// The tenant the StorageClass override names, which must be a different one for the
	// comparison below to say anything.
	storageClassTenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NotEqual(t, namespaceTenantID, storageClassTenantID,
		"QUOBYTE_TENANT resolves to the same tenant as the namespace, so the override cannot be told apart from the mapping")

	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, namespaceTenantID))

	// --- the mapping: no tenant in the StorageClass ---------------------------
	mappedTenantUUID := provisionAndResolveTenant(t, ctx, clientset, cfg, tenantCase{
		storageClassName: fmt.Sprintf("e2e-nstenant-sc-mapped-%d", suffix),
		tenant:           "", // left out on purpose: this is what enables the mapping
		secretName:       secretName,
		nameSuffix:       fmt.Sprintf("mapped-%d", suffix),
	})
	require.Equal(t, namespaceTenantID, mappedTenantUUID,
		"a claim through a StorageClass without quobyteTenant did not land in the tenant named after its namespace %q", cfg.Namespace)

	// --- the override: the StorageClass names a tenant ------------------------
	overriddenTenantUUID := provisionAndResolveTenant(t, ctx, clientset, cfg, tenantCase{
		storageClassName: fmt.Sprintf("e2e-nstenant-sc-override-%d", suffix),
		tenant:           cfg.QuobyteTenant,
		secretName:       secretName,
		nameSuffix:       fmt.Sprintf("override-%d", suffix),
	})
	require.Equal(t, storageClassTenantID, overriddenTenantUUID,
		"a claim through a StorageClass naming quobyteTenant=%q landed elsewhere, so the namespace mapping overrode it", cfg.QuobyteTenant)
}

// tenantCase is one StorageClass and the claim provisioned through it.
type tenantCase struct {
	storageClassName string
	tenant           string
	secretName       string
	nameSuffix       string
}

// provisionAndResolveTenant creates the StorageClass and a claim through it, and returns
// the tenant UUID the driver put into the bound PV's volume handle -- which is the only
// place the decision it made is visible.
func provisionAndResolveTenant(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, cfg framework.Config, testCase tenantCase) string {
	t.Helper()

	pvcName := fmt.Sprintf("e2e-nstenant-pvc-%s", testCase.nameSuffix)

	framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:            testCase.storageClassName,
		Provisioner:     cfg.CSIProvisionerName,
		Tenant:          testCase.tenant,
		SecretName:      testCase.secretName,
		SecretNamespace: cfg.Namespace,
	}))

	framework.ApplyPVC(t, ctx, clientset, framework.NewPVC(pvcName, cfg.Namespace, testCase.storageClassName, "1Gi"))

	boundPVC, err := framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, pvcName, 3*time.Minute)
	require.NoError(t, err, "waiting for pvc %s to bind", pvcName)

	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, boundPVC.Spec.VolumeName, metav1.GetOptions{})
	require.NoError(t, err, "fetching the PV bound to %s", pvcName)
	require.NotNil(t, pv.Spec.CSI, "PV %s has no CSI volume source", pv.Name)

	// The driver resolves whichever tenant it chose to a UUID before building the handle.
	parts := strings.Split(pv.Spec.CSI.VolumeHandle, "|")
	require.GreaterOrEqual(t, len(parts), 2, "unexpected volume handle format %q", pv.Spec.CSI.VolumeHandle)

	return parts[0]
}
