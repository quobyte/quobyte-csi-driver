package framework

import (
	"fmt"

	"github.com/quobyte/api/v4/quobyte"
)

// CreateUser creates a Quobyte user with the given password and makes it an
// admin of every tenant in adminOfTenantIDs -- enough for that user's
// credentials to be handed to the CSI driver in a Secret and used to provision
// volumes in those tenants.
func CreateUser(client *quobyte.QuobyteClient, userName, primaryGroup, password string,
	userRole quobyte.UserRole, adminOfTenantIDs []string) error {
	_, err := client.CreateUser(&quobyte.CreateUserRequest{
		UserName:         userName,
		Password:         password,
		Role: userRole,
		AdminOfTenantId:  adminOfTenantIDs,
		MemberOfTenantId: adminOfTenantIDs,
		PrimaryGroup:     primaryGroup,
		MemberOfGroup:    []string{primaryGroup},
	})
	if err != nil {
		return fmt.Errorf("creating user %q: %w", userName, err)
	}

	return nil
}

// DeleteUser removes the named Quobyte user, undoing CreateUser.
func DeleteUser(client *quobyte.QuobyteClient, userName string) error {
	if _, err := client.DeleteUser(&quobyte.DeleteUserRequest{UserName: userName}); err != nil {
		return fmt.Errorf("deleting user %q: %w", userName, err)
	}

	return nil
}
