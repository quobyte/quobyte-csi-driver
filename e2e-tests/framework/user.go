package framework

import (
	"fmt"
	"slices"

	"github.com/quobyte/api/v4/quobyte"
)

// CreateUser creates a Quobyte user with the given password and makes it an
// admin of every tenant in adminOfTenantIDs -- enough for that user's
// credentials to be handed to the CSI driver in a Secret and used to provision
// volumes in those tenants.
//
// Repeated tenants are dropped, keeping the first occurrence: the API stores the lists as
// they are given, and this one slice becomes both the user's admin-of and member-of tenants,
// so a caller that names a tenant twice -- easily done where two roles in a test happen to
// fall on the same tenant -- would end up with it listed twice in each. Order is kept
// because callers rely on it: NewCredentials issues the user's access key in the first
// tenant of the list.
func CreateUser(client *quobyte.QuobyteClient, userName, primaryGroup, password string,
	userRole quobyte.UserRole, adminOfTenantIDs []string) error {
	tenantIDs := distinct(adminOfTenantIDs)

	_, err := client.CreateUser(&quobyte.CreateUserRequest{
		UserName:         userName,
		Password:         password,
		Role:             userRole,
		AdminOfTenantId:  tenantIDs,
		MemberOfTenantId: tenantIDs,
		PrimaryGroup:     primaryGroup,
		MemberOfGroup:    []string{primaryGroup},
	})
	if err != nil {
		return fmt.Errorf("creating user %q: %w", userName, err)
	}

	return nil
}

// distinct returns values without repetitions, in the order they first appear.
func distinct(values []string) []string {
	deduplicated := make([]string, 0, len(values))
	for _, value := range values {
		if !slices.Contains(deduplicated, value) {
			deduplicated = append(deduplicated, value)
		}
	}

	return deduplicated
}

// DeleteUser removes the named Quobyte user, undoing CreateUser.
func DeleteUser(client *quobyte.QuobyteClient, userName string) error {
	if _, err := client.DeleteUser(&quobyte.DeleteUserRequest{UserName: userName}); err != nil {
		return fmt.Errorf("deleting user %q: %w", userName, err)
	}

	return nil
}
