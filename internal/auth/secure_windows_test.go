//go:build windows

package auth

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestSecureDirectoryPreservesOwnerAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := secureDir(dir); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("reopen inherited lock: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := secureSecretFile(path); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{dir, path} {
		sd, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		acl, _, err := sd.DACL()
		if err != nil || acl == nil {
			t.Fatalf("missing restricted DACL: %v", err)
		}
		if acl.AceCount == 0 {
			t.Fatal("DACL grants no owner access")
		}
		for index := uint32(0); index < uint32(acl.AceCount); index++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(acl, index, &ace); err != nil {
				t.Fatal(err)
			}
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !sid.Equals(user.User.Sid) {
				t.Fatal("DACL does not exclusively grant current user access")
			}
		}
	}
}
