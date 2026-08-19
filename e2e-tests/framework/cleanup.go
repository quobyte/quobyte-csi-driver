package framework

import "testing"

// CleanupUnlessFailed registers fn to run during t.Cleanup, but skips it if the
// test has already failed -- leaving whatever the test created (k8s resources as
// well as the Quobyte tenants/users/access keys it set up) in place, alongside
// the CSI driver/client test_runner deployed for this test, so a failure can be
// debugged live with kubectl against the still-running KUBECONFIG cluster
// instead of everything being torn down immediately.
func CleanupUnlessFailed(t *testing.T, description string, fn func()) {
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("test failed; leaving %s in place for debugging", description)
			return
		}
		fn()
	})
}
