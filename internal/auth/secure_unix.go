//go:build !windows

package auth

import (
	"fmt"
	"os"
	"syscall"
)

// openSecretFile opens a credential file without following a symlink.
func openSecretFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}

// checkPermissions refuses a credential file readable or writable by group or
// others, so a pre-existing loose file is never silently trusted.
func checkPermissions(info os.FileInfo, path string) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("credential file %s must not be accessible by group or others; use chmod 600", path)
	}
	return nil
}

// secureSecretFile enforces owner-only access on the credential file.
func secureSecretFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict credential file permissions: %w", err)
	}
	return nil
}

// secureDir enforces owner-only access on the credential directory.
func secureDir(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("restrict credential directory permissions: %w", err)
	}
	return nil
}

// checkOwner refuses a credential file owned by another user.
func checkOwner(info os.FileInfo, path string) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("credential file %s is owned by uid %d, not the current user", path, stat.Uid)
	}
	return nil
}
