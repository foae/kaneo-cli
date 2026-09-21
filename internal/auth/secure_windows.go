//go:build windows

package auth

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// openSecretFile opens a credential file for reading.
func openSecretFile(path string) (*os.File, error) {
	return os.Open(path)
}

// checkPermissions is a no-op on Windows; the DACL enforced by secureSecretFile
// is the protection.
func checkPermissions(_ os.FileInfo, _ string) error { return nil }

// secureSecretFile replaces the credential file's DACL with one that grants
// only the current user full control, protecting the token without relying on
// chmod semantics. Native Windows evidence is still required to confirm it.
func secureSecretFile(path string) error {
	return applyCurrentUserDACL(path)
}

// secureDir restricts the credential directory to the current user.
func secureDir(path string) error {
	return applyCurrentUserDACL(path)
}

// checkOwner is a no-op on Windows; the DACL set by secureSecretFile is the
// enforced protection.
func checkOwner(_ os.FileInfo, _ string) error { return nil }

func applyCurrentUserDACL(path string) error {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("resolve current user: %w", err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		return fmt.Errorf("build credential access control list: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	); err != nil {
		return fmt.Errorf("restrict credential file access: %w", err)
	}
	return nil
}
