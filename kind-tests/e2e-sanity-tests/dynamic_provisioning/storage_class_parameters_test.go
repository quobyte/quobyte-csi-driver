package dynamicprovisioning

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
	"k8s.io/client-go/rest"
)

const (
	paramsMountPath = "/mnt/test"
	// How long "it did not provision" has to hold before the test believes it. The
	// provisioner retries with backoff, so this has to outlast a couple of attempts.
	staysUnboundFor = 45 * time.Second
	// The identity the "user"/"group" case asks for. Deliberately a name every image
	// involved knows -- busybox and the kind node both have daemon at 1:1 -- because the
	// only place a volume's root owner can be observed is the mount, and there it is a
	// numeric id.
	ownerUser  = "1"
	ownerGroup = "1"
	ownerUID   = "1"
	ownerGID   = "1"
)

// TestStorageClassParameters covers the StorageClass parameters CreateVolume understands but
// nothing else in this suite ever sets: user, group, labels, accessMode and createQuota (see
// the parameter loop in src/driver/controller.go). Only quobyteTenant, sharedVolumeName and
// createQuota=true were exercised before, so a regression in any of the others would have
// shown up as silently wrong volumes -- owned by the API user, unlabelled, world-readable,
// or unbounded -- rather than as a failing test.
//
// Each case builds its own StorageClass over the one Secret created here, so they differ in
// nothing but the parameter under test.
func TestStorageClassParameters(t *testing.T) {
	cfg := framework.LoadConfig(t)
	ctx := context.Background()

	if cfg.SharedVolumeOptions.EnableSharedVolume {
		t.Skip("these cases are about the volume-per-claim parameters; the shared volume side of " +
			"accessMode is covered by shared_volume/")
	}

	clientset, restConfig, err := framework.NewClientset(cfg.Kubeconfig)
	require.NoError(t, err, "building kubernetes clientset")

	quobyteClient := framework.NewQuobyteClient(cfg)

	tenantID, err := framework.EnsureTenant(quobyteClient, cfg.QuobyteTenant, cfg.QuobyteAPIUser)
	require.NoError(t, err, "ensuring tenant %q exists and %q has access to it", cfg.QuobyteTenant, cfg.QuobyteAPIUser)

	suffix := time.Now().UnixNano()
	secretName := fmt.Sprintf("e2e-params-secret-%d", suffix)
	framework.ApplySecret(t, ctx, clientset,
		framework.NewCredentialsSecret(t, cfg, quobyteClient, secretName, tenantID))

	suite := &parameterSuite{
		cfg:           cfg,
		clientset:     clientset,
		restConfig:    restConfig,
		quobyteClient: quobyteClient,
		tenantID:      tenantID,
		secretName:    secretName,
	}

	t.Run("user and group set the volume's root owner", func(t *testing.T) {
		claim := suite.provision(t, ctx, "owner", map[string]string{
			"user":  ownerUser,
			"group": ownerGroup,
		})

		owner := suite.statMount(t, ctx, claim.podName, "%u:%g")
		require.Equal(t, ownerUID+":"+ownerGID, owner,
			"the volume's root is owned by %q, not by the user/group the StorageClass asked for "+
				"(%s/%s). Either the driver dropped the parameters, or this installation does not "+
				"map those names to the ids the mount reports", owner, ownerUser, ownerGroup)
	})

	t.Run("an empty group is refused rather than guessed", func(t *testing.T) {
		// RootGroupId is overwritten unconditionally by the "group" parameter, so an empty
		// one leaves the request without a primary group -- which CreateVolume refuses
		// outright instead of falling back to the API user's group.
		suite.expectProvisioningRefused(t, ctx, "emptygroup", map[string]string{
			"group": "",
		}, "primary group is empty")
	})

	t.Run("labels reach the Quobyte volume", func(t *testing.T) {
		labelName := fmt.Sprintf("e2e-label-%d", suffix)
		claim := suite.provision(t, ctx, "labels", map[string]string{
			"labels": labelName + ":e2e-value",
		})

		labels, err := framework.GetVolumeLabels(quobyteClient, claim.volumeUUID)
		require.NoError(t, err, "reading the labels of volume %s", claim.volumeUUID)
		require.Equal(t, "e2e-value", labels[labelName],
			"the volume does not carry the label the StorageClass asked for; it has %v", labels)
	})

	t.Run("a malformed label fails provisioning", func(t *testing.T) {
		// parseLabels (src/driver/utils.go) wants <name>:<value>; anything else is an error
		// that aborts CreateVolume, which must not leave a half-configured volume behind.
		suite.expectProvisioningRefused(t, ctx, "badlabel", map[string]string{
			"labels": "no-value-here",
		}, "label")
	})

	t.Run("accessMode sets the volume's root mode", func(t *testing.T) {
		// 770 rather than the driver's default of 700 (DefaultAccessModes), so the assertion
		// cannot pass on the default.
		claim := suite.provision(t, ctx, "accessmode", map[string]string{
			"accessMode": "770",
		})

		mode := suite.statMount(t, ctx, claim.podName, "%a")
		require.Equal(t, "770", mode,
			"the volume's root has mode %s, not the 770 the StorageClass asked for", mode)
	})

	t.Run("createQuota decides whether the volume gets a quota", func(t *testing.T) {
		withQuota := suite.provision(t, ctx, "quota-on", map[string]string{
			"createQuota": "true",
		})
		quota, hasQuota, err := framework.GetVolumeQuota(quobyteClient, withQuota.volumeUUID)
		require.NoError(t, err, "reading the quota of volume %s", withQuota.volumeUUID)
		require.True(t, hasQuota,
			"the claim asked for 1Gi through a StorageClass with createQuota=true, but the volume has no quota")
		require.Equal(t, int64(1<<30), quota,
			"the volume's quota does not match the 1Gi the claim asked for")

		withoutQuota := suite.provision(t, ctx, "quota-off", map[string]string{
			"createQuota": "false",
		})
		_, hasQuota, err = framework.GetVolumeQuota(quobyteClient, withoutQuota.volumeUUID)
		require.NoError(t, err, "reading the quota of volume %s", withoutQuota.volumeUUID)
		require.False(t, hasQuota,
			"createQuota=false still put a quota on the volume, so the parameter is ignored")
	})

	t.Run("a quota that cannot be set leaves no volume behind", func(t *testing.T) {
		// The one error path CreateVolume cleans up after: the volume is created first and
		// the quota set second, so a rejected quota has to take the volume with it
		// (src/driver/controller.go) -- otherwise every failed claim leaks a volume.
		//
		// A tenant of its own, capped and not oversubscribable, both to provoke the failure
		// and to make "no volume was left behind" an exact statement rather than a diff
		// against whatever else this test has provisioned.
		tenantName := fmt.Sprintf("%s-rollback-%d", cfg.QuobyteTenant, suffix)
		rollbackTenantID, err := framework.EnsureTenant(quobyteClient, tenantName, cfg.QuobyteAPIUser)
		require.NoError(t, err, "creating tenant %q for the rollback case", tenantName)
		framework.CleanupUnlessFailed(t, fmt.Sprintf("tenant %s", tenantName), func() {
			if _, err := framework.DeleteVolumesInTenant(quobyteClient, rollbackTenantID); err != nil {
				t.Logf("warning: %v", err)
			}
			if err := framework.DeleteTenant(quobyteClient, tenantName); err != nil {
				t.Logf("warning: %v", err)
			}
		})

		const tenantLimit = int64(1 << 30) // 1Gi, less than the claim below asks for
		require.NoError(t, framework.CapTenantDiskSpace(quobyteClient, rollbackTenantID, tenantLimit),
			"capping tenant %q so an oversized volume quota has to be refused", tenantName)

		rollbackSecret := fmt.Sprintf("e2e-params-rollback-secret-%d", suffix)
		framework.ApplySecret(t, ctx, clientset,
			framework.NewCredentialsSecret(t, cfg, quobyteClient, rollbackSecret, rollbackTenantID))

		names := suite.namesFor("rollback")
		framework.ApplyStorageClass(t, ctx, clientset, framework.NewStorageClass(framework.StorageClassOptions{
			Name:            names.storageClassName,
			Provisioner:     cfg.CSIProvisionerName,
			Tenant:          tenantName,
			SecretName:      rollbackSecret,
			SecretNamespace: cfg.Namespace,
			ExtraParameters: map[string]string{"createQuota": "true"},
		}))
		framework.ApplyPVC(t, ctx, clientset,
			framework.NewPVC(names.pvcName, cfg.Namespace, names.storageClassName, "10Gi"))

		require.NoError(t, framework.EnsurePVCStaysUnbound(ctx, clientset, cfg.Namespace, names.pvcName, staysUnboundFor),
			"the claim asked for 10Gi from a tenant capped at 1Gi that may not be oversubscribed, "+
				"and was provisioned anyway")

		volumeNames, err := framework.ListVolumeNamesInTenant(quobyteClient, rollbackTenantID)
		require.NoError(t, err, "listing the volumes of tenant %q", tenantName)
		require.Empty(t, volumeNames,
			"setting the quota failed but the volume created just before it was left behind: %v", volumeNames)
	})
}

// parameterSuite carries what every case needs, so each case reads as the parameter it is
// about.
type parameterSuite struct {
	cfg           framework.Config
	clientset     *kubernetes.Clientset
	restConfig    *rest.Config
	quobyteClient *quobyte.QuobyteClient
	tenantID      string
	secretName    string
}

// resourceNames are the k8s object names of one case, unique to it so cases cannot collide.
type resourceNames struct {
	storageClassName string
	pvcName          string
	podName          string
}

// provisionedClaim is a bound claim of one case, with the Quobyte volume behind it.
type provisionedClaim struct {
	podName    string
	volumeUUID string
}

func (s *parameterSuite) namesFor(caseName string) resourceNames {
	suffix := fmt.Sprintf("%s-%d", caseName, time.Now().UnixNano())

	return resourceNames{
		storageClassName: "e2e-params-sc-" + suffix,
		pvcName:          "e2e-params-pvc-" + suffix,
		podName:          "e2e-params-pod-" + suffix,
	}
}

// provision creates a StorageClass carrying the given extra parameters, a claim against it
// and a pod mounting that claim, and returns once all three are up. The Quobyte volume it
// resolves is the one the assertions are about.
func (s *parameterSuite) provision(t *testing.T, ctx context.Context, caseName string, params map[string]string) provisionedClaim {
	t.Helper()

	names := s.namesFor(caseName)

	framework.ApplyStorageClass(t, ctx, s.clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:            names.storageClassName,
		Provisioner:     s.cfg.CSIProvisionerName,
		Tenant:          s.cfg.QuobyteTenant,
		SecretName:      s.secretName,
		SecretNamespace: s.cfg.Namespace,
		ExtraParameters: params,
	}))

	framework.ApplyPVC(t, ctx, s.clientset,
		framework.NewPVC(names.pvcName, s.cfg.Namespace, names.storageClassName, "1Gi"))
	boundPVC, err := framework.WaitForPVCBound(ctx, s.clientset, s.cfg.Namespace, names.pvcName, 3*time.Minute)
	require.NoErrorf(t, err, "waiting for the claim of the %q case to bind", caseName)

	framework.ApplyPod(t, ctx, s.clientset,
		framework.NewPod(names.podName, s.cfg.Namespace, names.pvcName, paramsMountPath))
	_, err = framework.WaitForPodRunning(ctx, s.clientset, s.cfg.Namespace, names.podName, 2*time.Minute)
	require.NoErrorf(t, err, "waiting for the pod of the %q case to run", caseName)

	_, volumeUUID, _, err := splitVolumeHandle(volumeHandleOfPV(t, ctx, s.clientset, boundPVC.Spec.VolumeName))
	require.NoError(t, err, "parsing the volume handle of the %q case", caseName)

	return provisionedClaim{podName: names.podName, volumeUUID: volumeUUID}
}

// expectProvisioningRefused asserts that a StorageClass carrying these parameters does not
// provision at all, and that Kubernetes is told why: a parameter the driver rejects must
// fail visibly rather than quietly produce a volume configured some other way.
func (s *parameterSuite) expectProvisioningRefused(t *testing.T, ctx context.Context, caseName string,
	params map[string]string, wantInEvent string) {
	t.Helper()

	names := s.namesFor(caseName)

	framework.ApplyStorageClass(t, ctx, s.clientset, framework.NewStorageClass(framework.StorageClassOptions{
		Name:            names.storageClassName,
		Provisioner:     s.cfg.CSIProvisionerName,
		Tenant:          s.cfg.QuobyteTenant,
		SecretName:      s.secretName,
		SecretNamespace: s.cfg.Namespace,
		ExtraParameters: params,
	}))
	framework.ApplyPVC(t, ctx, s.clientset,
		framework.NewPVC(names.pvcName, s.cfg.Namespace, names.storageClassName, "1Gi"))

	require.NoErrorf(t, framework.EnsurePVCStaysUnbound(ctx, s.clientset, s.cfg.Namespace, names.pvcName, staysUnboundFor),
		"the %q case was provisioned although the driver should have refused its parameters", caseName)

	message, err := framework.WaitForEventMatching(ctx, s.clientset, s.cfg.Namespace, names.pvcName, wantInEvent, time.Minute)
	require.NoErrorf(t, err,
		"the claim of the %q case never bound, but nothing said %q -- the reason has to reach the user",
		caseName, wantInEvent)
	t.Logf("provisioning was refused with: %s", message)
}

// volumeHandleOfPV returns the CSI volume handle of a PV, the driver's own name for what it
// provisioned (see CreateVolume in src/driver/controller.go).
func volumeHandleOfPV(t *testing.T, ctx context.Context, clientset *kubernetes.Clientset, pvName string) string {
	t.Helper()

	pv, err := clientset.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	require.NoError(t, err, "fetching PV %s", pvName)
	require.NotNil(t, pv.Spec.CSI, "PV %s has no CSI volume source", pvName)

	return pv.Spec.CSI.VolumeHandle
}

// statMount returns one stat format field of the mount point, as seen from inside the pod.
// The volume's root owner and mode are not readable through the Quobyte API, so the mount is
// where they have to be observed.
func (s *parameterSuite) statMount(t *testing.T, ctx context.Context, podName, format string) string {
	t.Helper()

	stdout, stderr, err := framework.ExecInPod(ctx, s.restConfig, s.clientset, s.cfg.Namespace, podName,
		framework.TestContainerName, []string{"stat", "-c", format, paramsMountPath})
	require.NoErrorf(t, err, "stat -c %s %s inside pod %s: stderr=%s", format, paramsMountPath, podName, stderr)

	return strings.TrimSpace(stdout)
}
