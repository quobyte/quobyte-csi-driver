package accesskeynegative

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quobyte/quobyte-csi-driver/kind-tests/framework"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
)

const (
	mountPath = "/mnt/test"
	// How long "it did not work" has to hold before the test believes it. Long enough for
	// the provisioner and the kubelet to have tried and retried, short enough that two of
	// these still fit comfortably inside the default go test timeout.
	stayFailedFor = 90 * time.Second
)

// TestBadAccessKeyIsRejected covers the "Access key negative tests" entry of the TODO list
// in kind-tests/e2e-sanity-tests/README.md.
//
// Every other test here asserts that the right credentials work. This one asserts the other
// direction, which is what makes access keys worth having: wrong or missing credentials
// must fail, visibly, rather than quietly falling back to something that works. Both cases
// below therefore assert two things -- that the volume never became usable, and that
// Kubernetes was told why.
//
// The driver puts a Secret to two uses, and the cases split along that line:
//
//	user/password    accepted for the API (src/driver/quobyte_api_client_factory.go), so
//	                 provisioning succeeds and the failure lands at mount time, where the
//	                 node plugin requires accessKeyId/accessKeySecret (src/driver/node.go)
//	unknown key      rejected by the API, so provisioning itself never succeeds
func TestBadAccessKeyIsRejected(t *testing.T) {
	cfg := framework.LoadConfig(t)

	require.True(t, cfg.EnableAccessKeyMounts,
		"this test only makes sense against a driver deployed with access key mounts; check this directory's env file")

	t.Run("user password instead of an access key fails at mount time", func(t *testing.T) {
		ctx := context.Background()
		clientset, _, err := framework.NewClientset(cfg.Kubeconfig)
		require.NoError(t, err, "building kubernetes clientset")

		// Provisioning is supposed to succeed in this case, so the tenant has to be there.
		_, err = framework.EnsureTenant(framework.NewQuobyteClient(cfg), cfg.QuobyteTenant, cfg.QuobyteAPIUser)
		require.NoError(t, err, "ensuring tenant %q exists", cfg.QuobyteTenant)

		names := newNames(t, "userpass")

		// Valid credentials, wrong kind: enough to provision, not enough to mount.
		framework.ApplySecret(t, ctx, clientset,
			framework.NewSecret(names.secret, cfg.Namespace, cfg.QuobyteAPIUser, cfg.QuobyteAPIPassword))
		applyStorageClassAndClaim(t, ctx, clientset, cfg, names)

		_, err = framework.WaitForPVCBound(ctx, clientset, cfg.Namespace, names.pvc, 3*time.Minute)
		require.NoError(t, err, "the PVC should still provision -- user/password are valid API credentials")

		framework.ApplyPod(t, ctx, clientset, framework.NewPod(names.pod, cfg.Namespace, names.pvc, mountPath))

		require.NoError(t, framework.EnsurePodStaysNotRunning(ctx, clientset, cfg.Namespace, names.pod, stayFailedFor),
			"the pod mounted a Quobyte volume with a secret that carries no access key")

		message, err := framework.WaitForEventMatching(ctx, clientset, cfg.Namespace, names.pod, "accessKeyId", time.Minute)
		require.NoError(t, err,
			"the pod never ran, but nothing said the mount secret was missing its access key")
		t.Logf("mount was refused with: %s", message)
	})

	t.Run("unknown access key fails at provisioning time", func(t *testing.T) {
		ctx := context.Background()
		clientset, _, err := framework.NewClientset(cfg.Kubeconfig)
		require.NoError(t, err, "building kubernetes clientset")

		names := newNames(t, "unknownkey")

		// Well-formed but never issued by this Quobyte installation.
		framework.ApplySecret(t, ctx, clientset,
			framework.NewAccessKeySecret(names.secret, cfg.Namespace, "e2eNoSuchAccessKeyId", "e2eNoSuchAccessKeySecret"))
		applyStorageClassAndClaim(t, ctx, clientset, cfg, names)

		require.NoError(t, framework.EnsurePVCStaysUnbound(ctx, clientset, cfg.Namespace, names.pvc, stayFailedFor),
			"a volume was provisioned with an access key this installation never issued")

		message, err := framework.WaitForEventMatching(ctx, clientset, cfg.Namespace, names.pvc, "failed to provision volume", time.Minute)
		require.NoError(t, err,
			"the PVC never bound, but no provisioning failure was reported on it")
		t.Logf("provisioning was refused with: %s", message)
	})
}

// resourceNames are the names one case gives its own resources, kept unique per case so the
// two subtests cannot collide in the shared namespace.
type resourceNames struct {
	secret       string
	storageClass string
	pvc          string
	pod          string
}

func newNames(t *testing.T, caseName string) resourceNames {
	t.Helper()

	suffix := fmt.Sprintf("%s-%d", caseName, time.Now().UnixNano())

	return resourceNames{
		secret:       "e2e-akneg-secret-" + suffix,
		storageClass: "e2e-akneg-sc-" + suffix,
		pvc:          "e2e-akneg-pvc-" + suffix,
		pod:          "e2e-akneg-pod-" + suffix,
	}
}

func applyStorageClassAndClaim(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, cfg framework.Config, names resourceNames) {
	t.Helper()

	framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:            names.storageClass,
		Provisioner:     cfg.CSIProvisionerName,
		Tenant:          cfg.QuobyteTenant,
		SecretName:      names.secret,
		SecretNamespace: cfg.Namespace,
	}))
	framework.ApplyPVC(t, ctx, clientset, framework.NewPVC(names.pvc, cfg.Namespace, names.storageClass, "1Gi"))
}
