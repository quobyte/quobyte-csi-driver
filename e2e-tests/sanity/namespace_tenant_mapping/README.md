# Tenant selection

Which Quobyte tenant a dynamically provisioned volume lands in. The decision is made by
`CreateVolume` in [`src/driver/controller.go`](../../../src/driver/controller.go) out of
three candidates:

| Candidate | Applies when |
| --- | --- |
| The StorageClass's `quobyteTenant` | Always, wherever it is set — it overrides the other two |
| The PVC's Kubernetes namespace | The driver runs with `quobyte.useK8SNamespaceAsTenant=true` and the StorageClass names no tenant. The driver does **not** create tenants: one named after the namespace has to exist already |
| The credentials' own tenant | Neither of the above names one. The driver then sends no tenant at all and the Quobyte API resolves it from the access key in the Secret |

So the namespace mapping and the access key both only ever supply a *default*, and the
StorageClass always wins over whichever default is in play.

## What each environment covers

The `env/` files cross access key mounts with the namespace mapping, and
[`namespace_tenant_mapping_test.go`](./namespace_tenant_mapping_test.go) runs once per file:

| env file | Credentials | Mapping | Default tenant (no `quobyteTenant`) | With `quobyteTenant` |
| --- | --- | --- | --- | --- |
| `user_password` | user/password | on | the namespace's tenant | the StorageClass's |
| `access_keys` | access key | on | the namespace's tenant — the key's tenant is *not* used | the StorageClass's |
| `access_keys_no_namespace_mapping` | access key | off | the access key's tenant | the StorageClass's |
| `user_password_no_namespace_mapping` | user/password | off | undefined, not asserted | the StorageClass's |

The last row is the one case the driver promises nothing about: nothing names a tenant, so
it sends none and a user/password pair belongs to no single tenant. The test says so in its
output and runs only the override case there.

## How a run tells the rules apart

Up to three tenants exist during a run, deliberately distinct, so the tenant a volume ends
up in names the rule that put it there and no other:

| Tenant | Where it comes from | Created for |
| --- | --- | --- |
| `$QUOBYTE_TENANT` | generated per run by `test_runner`, shared with the other sanity tests | the StorageClass override |
| `$NAMESPACE` | the test's own randomized namespace name | the namespace mapping |
| `e2e-nstenant-akey-<ts>` | invented by the test | the access key to be issued in |

The two cases of a run differ in nothing but their StorageClass, and the tenant is read back
out of the bound PV's volume handle, `<tenant>|<volume>`.

With one exception: the handle carries the tenant *the driver resolved*, and in
`access_keys_no_namespace_mapping` the driver resolves none — it sends no tenant and the
Quobyte API picks the access key's own. The handle then begins with an empty first part, and
the test asks Quobyte which tenant the volume ended up in
(`framework.TenantOfVolume`, by the volume UUID from the handle's second part). That is not a
fallback for a broken handle; it is the only way to observe the one rule where the decision is
made on the storage side rather than in the driver.

The credentials in the Secret are *not* the environment's API user. **Each case creates a
Quobyte user and Secret of its own** (`framework.CreateTestUser`), admin of only the tenants
that case needs: the one it expects the volume to land in, plus — where access key mounts are
on — the tenant the key is issued in. The Secret carries that user's password, or an access
key issued for it.

Nothing here grants an already-existing user anything. The tenants are created first
(`framework.EnsureTenantExists`) and named in `CreateUser`, so a case's user is admin of them
from the moment it exists. Granting instead would mean updating a user, and an update replaces
that user's tenant mappings wholesale: doing it to the API user, which every test of the run
shares, is a side effect on the other tests and something to undo afterwards, and the mappings
go stale as tests delete their tenants. Two further reasons not to reuse the API user: an
access key needs a user with a primary group, which an installation's API user need not have,
and a user that is only an unprivileged tenant admin is what shows a tenant admin is enough to
drive the driver.

Per-case credentials also make each assertion narrower. In `access_keys`, the fallback case's
user is admin of the namespace's tenant *and* of the key's own, so "the volume landed in the
namespace's tenant" says the mapping won rather than that nothing else was reachable. In
`access_keys_no_namespace_mapping` those two are the same tenant — the key's own tenant *is*
the fallback there — and the case's user is admin of it once, not twice.

The test creates the last two and gives them back afterwards; `$QUOBYTE_TENANT` is left
behind, as everywhere else in the sanity set. Tenant deletion happens while the volumes
provisioned into them may still exist, so a `warning: deleting tenant ...` line in the output
is expected and does not fail the run.

## Running it

```bash
TESTS='namespace_tenant_mapping' e2e-tests/test_runner --sanity http://host:port host:port
```

Each env file is a full driver redeploy, so that is four deploy/run/undeploy cycles. Name a
single file (`TESTS='namespace_tenant_mapping/access_keys'`) to iterate on one of them — see
[../../README.md](../../README.md) for running a test against an already-deployed cluster.
