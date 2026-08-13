package framework

import (
	"fmt"
	"slices"

	"github.com/quobyte/api/v4/quobyte"
)

// EnsureTenantExists makes sure a tenant named tenantName exists, creating it via the
// Quobyte API if it does not, and returns its UUID. Idempotent.
//
// Unlike EnsureTenant it grants nobody anything, so no existing user is touched. Use it
// where the test hands the tenant to a user it creates for itself -- CreateUser takes the
// tenants a new user is admin of, so a user made after the tenant needs no grant at all, and
// nothing shared has to be rewritten to give one test access to its own tenant.
func EnsureTenantExists(client *quobyte.QuobyteClient, tenantName string) (string, error) {
	if tenantID, err := client.ResolveTenantNameToUUID(tenantName); err == nil {
		return tenantID, nil
	}

	createResp, err := client.SetTenant(&quobyte.SetTenantRequest{
		Tenant: quobyte.TenantDomainConfiguration{Name: tenantName},
	})
	if err != nil {
		return "", fmt.Errorf("creating tenant %q: %w", tenantName, err)
	}

	return createResp.TenantId, nil
}

// EnsureTenant makes sure a tenant named tenantName exists (creating it via the
// Quobyte API if it doesn't) and that quobyteUser has admin access to it, and
// returns the tenant's UUID. Idempotent -- safe to call on every test run
// whether tenantName is freshly generated or refers to an already-existing,
// already-granted tenant.
//
// Granting rewrites that user's tenant mappings wholesale (see below), so this is only for
// tenants an already-existing user has to reach. A test whose own user is created after the
// tenant should use EnsureTenantExists and name the tenant in CreateUser instead.
func EnsureTenant(client *quobyte.QuobyteClient, tenantName, quobyteUser string) (string, error) {
	tenantID, err := EnsureTenantExists(client, tenantName)
	if err != nil {
		return "", err
	}

	usersResp, err := client.GetUsers(&quobyte.GetUsersRequest{UserId: []string{quobyteUser}})
	if err != nil {
		return "", fmt.Errorf("looking up user %q: %w", quobyteUser, err)
	}
	if len(usersResp.UserConfiguration) == 0 {
		return "", fmt.Errorf("user %q not found", quobyteUser)
	}
	user := usersResp.UserConfiguration[0]

	for _, adminOfTenantID := range user.AdminOfTenantId {
		if adminOfTenantID == tenantID {
			return tenantID, nil
		}
	}

	// An update replaces the user's tenant mappings wholesale, so the ones it already has
	// have to be sent along -- including any naming a tenant that no longer exists, which
	// the API rejects with "tenant not found". Tests create and delete tenants of their own
	// all the time and nothing prunes the mappings they leave behind on a shared user, so
	// they are dropped here, at the one point every test's setup passes through.
	existingTenants, err := existingTenantIDs(client)
	if err != nil {
		return "", err
	}

	updateReq := &quobyte.UpdateUserRequest{
		UserName:         quobyteUser,
		AdminOfTenantId:  append(keepExistingTenants(user.AdminOfTenantId, existingTenants), tenantID),
		Email:            user.Email,
		PrimaryGroup:     user.PrimaryGroup,
		MemberOfGroup:    user.Group,
		MemberOfTenantId: keepExistingTenants(user.MemberOfTenantId, existingTenants),
	}
	if len(user.Role) > 0 {
		updateReq.Role = *user.Role[0]
	}

	if _, err := client.UpdateUser(updateReq); err != nil {
		return "", fmt.Errorf("granting user %q access to tenant %q: %w", quobyteUser, tenantName, err)
	}

	return tenantID, nil
}

// existingTenantIDs returns the UUIDs of every tenant the installation currently has, as a
// set to test membership against.
func existingTenantIDs(client *quobyte.QuobyteClient) (map[string]bool, error) {
	resp, err := client.GetTenant(&quobyte.GetTenantRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}

	tenantIDs := make(map[string]bool, len(resp.Tenant))
	for _, tenant := range resp.Tenant {
		tenantIDs[tenant.TenantId] = true
	}

	return tenantIDs, nil
}

// keepExistingTenants drops the IDs of tenants that are gone, so a user's stale mappings do
// not travel back to the API in the next update.
//
// Repetitions are dropped too. An update rewrites these lists wholesale, so a user that
// already carries a tenant twice -- from a request that named it twice -- would carry it
// twice for the rest of its life otherwise, and every later update would write the duplicate
// back. Passing through here is the one point where that can be undone.
func keepExistingTenants(tenantIDs []string, existing map[string]bool) []string {
	kept := make([]string, 0, len(tenantIDs))
	for _, tenantID := range tenantIDs {
		if existing[tenantID] && !slices.Contains(kept, tenantID) {
			kept = append(kept, tenantID)
		}
	}

	return kept
}

// DeleteTenant removes the tenant named tenantName. Used by tests that created
// their own tenant during setup to give it back afterwards; a tenant that no
// longer exists is not an error.
func DeleteTenant(client *quobyte.QuobyteClient, tenantName string) error {
	tenantID, err := client.ResolveTenantNameToUUID(tenantName)
	if err != nil {
		return nil
	}

	if _, err := client.DeleteTenant(&quobyte.DeleteTenantRequest{TenantId: tenantID}); err != nil {
		return fmt.Errorf("deleting tenant %q (%s): %w", tenantName, tenantID, err)
	}

	return nil
}
