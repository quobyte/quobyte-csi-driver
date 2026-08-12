package namespacetenantmapping

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quobyte/api/v4/quobyte"
	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TestTenantSelection covers the "Namespace-to-tenant mapping tests (tenant overrides)"
// entry of the TODO list in kind-tests/e2e-sanity-tests/README.md.
//
// Which Quobyte tenant a dynamically provisioned volume lands in is decided by CreateVolume
// in src/driver/controller.go, out of three candidates:
//
//	the StorageClass's quobyteTenant  wins wherever it is set -- always an override
//	the PVC's namespace               where the driver runs with useK8SNamespaceAsTenant
//	                                  and the StorageClass names no tenant
//	the credentials' own tenant       where neither of the above names one: the driver
//	                                  sends no tenant at all and the Quobyte API resolves
//	                                  it from the access key in the Secret
//
// This package's env files are that matrix -- access key mounts on or off, crossed with the
// namespace mapping on or off -- and this test asserts, per environment, which of the rules
// applied. The override case runs in every environment. The case without a StorageClass
// tenant only runs where its outcome is defined, which rules out user/password without the
// mapping: nothing names a tenant then, the driver sends none, and what the API makes of
// that is not something the driver promises either way.
//
// The two cases of a run differ in nothing but their StorageClass, and the tenants involved
// are deliberately distinct ones, so the tenant a volume ends up in names the rule the
// driver applied and no other.
func TestTenantSelection(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	clientset, _, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-nstenant-secret-%d", suffix)

	// The tenant the StorageClass override names. Generated per run by run_test and shared
	// with every other sanity test, so -- unlike the tenants below, which this test invents
	// for itself -- it is left behind rather than deleted.
	storageClassTenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	// The mapping is namespace name -> tenant name and the driver does not create tenants,
	// so where the mapping is on, the tenant named after this run's namespace has to exist
	// first. It is this test's own (the namespace is generated per run by run_test), so it
	// goes again afterwards.
	namespaceTenantID := ""
	if cfg.UseK8SNamespaceAsTenant {
		namespaceTenantID = ensureTestTenant(t, quobyteClient, cfg, cfg.Namespace)
		t.Logf("tenant %s (%s) stands in for namespace %s", cfg.Namespace, namespaceTenantID, cfg.Namespace)
		require.NotEqual(t, storageClassTenantID, namespaceTenantID,
			"QUOBYTE_TENANT resolves to the same tenant as the namespace, so the override cannot be told apart from the mapping")
	}

	// Whom the Secret's credentials belong to. An access key is issued inside a tenant, so
	// give it one of its own: a volume landing there can then only mean the driver named no
	// tenant and left the choice to the API. Without access key mounts the Secret carries
	// the API user's user/password, which belong to no single tenant, and there is nothing
	// to set up.
	credentialsTenantID := ""
	if cfg.EnableAccessKeyMounts {
		credentialsTenantID = ensureTestTenant(t, quobyteClient, cfg, fmt.Sprintf("e2e-nstenant-akey-%d", suffix))
	}

	// After the tenant above, never before: NewCredentialsSecret registers the revocation of
	// the access key it creates, and LIFO cleanup then revokes the key before deleting the
	// tenant it was issued in.
	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, credentialsTenantID))

	// --- no tenant in the StorageClass: whichever fallback this environment has ---
	if fallbackTenantID, rule, defined := expectedFallbackTenant(cfg, namespaceTenantID, credentialsTenantID); defined {
		t.Run("without a storage class tenant", func(t *testing.T) {
			tenantUUID := provisionAndResolveTenant(t, ctx, clientset, cfg, tenantCase{
				storageClassName: fmt.Sprintf("e2e-nstenant-sc-fallback-%d", suffix),
				tenant:           "", // left out on purpose: this is what leaves the choice to the driver
				secretName:       secretName,
				nameSuffix:       fmt.Sprintf("fallback-%d", suffix),
			})
			require.Equal(t, fallbackTenantID, tenantUUID,
				"a claim through a StorageClass without quobyteTenant did not land in %s", rule)
		})
	} else {
		t.Log("not running the case without a StorageClass tenant: with user/password credentials " +
			"and no namespace mapping nothing names a tenant, and the driver promises no outcome")
	}

	// --- the override: the StorageClass names a tenant ------------------------
	t.Run("with a storage class tenant", func(t *testing.T) {
		tenantUUID := provisionAndResolveTenant(t, ctx, clientset, cfg, tenantCase{
			storageClassName: fmt.Sprintf("e2e-nstenant-sc-override-%d", suffix),
			tenant:           cfg.QuobyteTenant,
			secretName:       secretName,
			nameSuffix:       fmt.Sprintf("override-%d", suffix),
		})
		require.Equal(t, storageClassTenantID, tenantUUID,
			"a claim through a StorageClass naming quobyteTenant=%q landed in another tenant, so something overrode the override", cfg.QuobyteTenant)
	})
}

// expectedFallbackTenant returns the tenant a claim through a StorageClass that names none
// has to land in for cfg's environment, along with a description of the rule that puts it
// there. defined is false where no rule names a tenant at all -- user/password credentials
// without the namespace mapping -- and the case is then not worth running: the driver sends
// no tenant, and whether the API still finds one for it is undefined.
func expectedFallbackTenant(cfg framework.Config, namespaceTenantID, credentialsTenantID string) (tenantID, rule string, defined bool) {
	switch {
	case cfg.UseK8SNamespaceAsTenant:
		// Ahead of the access key deliberately: the mapping names a tenant, so it does not
		// matter that the credentials belong to one of their own.
		return namespaceTenantID, fmt.Sprintf("the tenant named after its namespace %q", cfg.Namespace), true
	case cfg.EnableAccessKeyMounts:
		return credentialsTenantID, "the tenant the Secret's access key belongs to", true
	default:
		return "", "", false
	}
}

// ensureTestTenant creates the named tenant if it does not exist yet, grants the API user
// access to it and registers its removal. For the tenants this test invents for itself --
// the driver creates none, so every tenant a case expects a volume to land in has to be
// there before the claim is.
func ensureTestTenant(t *testing.T, quobyteClient *quobyte.QuobyteClient, cfg framework.Config, tenantName string) string {
	t.Helper()

	tenantID, err := framework.EnsureTenant(quobyteClient, tenantName, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", tenantName, cfg.QuobyteAPIUser)

	framework.CleanupUnlessFailed(t, fmt.Sprintf("quobyte tenant %s", tenantName), func() {
		if err := framework.DeleteTenant(quobyteClient, tenantName); err != nil {
			t.Logf("warning: %v", err)
		}
	})

	return tenantID
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
