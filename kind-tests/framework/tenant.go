package framework

import (
	"fmt"

	"github.com/quobyte/api/v4/quobyte"
)

// EnsureTenant makes sure a tenant named tenantName exists (creating it via the
// Quobyte API if it doesn't) and that quobyteUser has admin access to it, and
// returns the tenant's UUID. Idempotent -- safe to call on every test run
// whether tenantName is freshly generated or refers to an already-existing,
// already-granted tenant.
func EnsureTenant(client *quobyte.QuobyteClient, tenantName, quobyteUser string) (string, error) {
	tenantID, err := client.ResolveTenantNameToUUID(tenantName)
	if err != nil {
		createResp, createErr := client.SetTenant(&quobyte.SetTenantRequest{
			Tenant: quobyte.TenantDomainConfiguration{Name: tenantName},
		})
		if createErr != nil {
			return "", fmt.Errorf("creating tenant %q: %w", tenantName, createErr)
		}
		tenantID = createResp.TenantId
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
func keepExistingTenants(tenantIDs []string, existing map[string]bool) []string {
	kept := make([]string, 0, len(tenantIDs))
	for _, tenantID := range tenantIDs {
		if existing[tenantID] {
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
