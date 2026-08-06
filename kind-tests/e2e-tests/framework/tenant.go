package framework

import (
	"fmt"

	"github.com/quobyte/api/v4/quobyte"
)

// EnsureTenant makes sure a tenant named tenantName exists (creating it via the
// Quobyte API if it doesn't) and that quobyteUser has admin access to it.
// Idempotent -- safe to call on every test run whether tenantName is freshly
// generated or refers to an already-existing, already-granted tenant.
func EnsureTenant(client *quobyte.QuobyteClient, tenantName, quobyteUser string) error {
	tenantID, err := client.ResolveTenantNameToUUID(tenantName)
	if err != nil {
		createResp, createErr := client.SetTenant(&quobyte.SetTenantRequest{
			Tenant: quobyte.TenantDomainConfiguration{Name: tenantName},
		})
		if createErr != nil {
			return fmt.Errorf("creating tenant %q: %w", tenantName, createErr)
		}
		tenantID = createResp.TenantId
	}

	usersResp, err := client.GetUsers(&quobyte.GetUsersRequest{UserId: []string{quobyteUser}})
	if err != nil {
		return fmt.Errorf("looking up user %q: %w", quobyteUser, err)
	}
	if len(usersResp.UserConfiguration) == 0 {
		return fmt.Errorf("user %q not found", quobyteUser)
	}
	user := usersResp.UserConfiguration[0]

	for _, adminOfTenantID := range user.AdminOfTenantId {
		if adminOfTenantID == tenantID {
			return nil
		}
	}

	updateReq := &quobyte.UpdateUserRequest{
		UserName:         quobyteUser,
		AdminOfTenantId:  append(user.AdminOfTenantId, tenantID),
		Email:            user.Email,
		PrimaryGroup:     user.PrimaryGroup,
		MemberOfGroup:    user.Group,
		MemberOfTenantId: user.MemberOfTenantId,
	}
	if len(user.Role) > 0 {
		updateReq.Role = *user.Role[0]
	}

	if _, err := client.UpdateUser(updateReq); err != nil {
		return fmt.Errorf("granting user %q access to tenant %q: %w", quobyteUser, tenantName, err)
	}

	return nil
}
