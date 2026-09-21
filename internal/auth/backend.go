// Package auth owns credential storage and the device authorization flow.
package auth

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/foae/kaneo-cli/internal/config"
)

// ErrNotStored reports that no credential exists for the requested key.
var ErrNotStored = errors.New("credential not stored")

// ErrKeyringUnavailable reports that the OS keyring cannot be reached.
var ErrKeyringUnavailable = errors.New("OS keyring unavailable")

// ErrCredentialMissing reports a recorded credential reference whose secret is
// absent from the backend, which must not be reported as "not logged in".
var ErrCredentialMissing = errors.New("stored credential is missing from its backend")

// Backend stores, retrieves and removes one secret per profile/URL key.
type Backend interface {
	Name() string
	Get(profile, apiURL string) (string, error)
	Set(profile, apiURL, secret string) error
	Delete(profile, apiURL string) error
	// Location describes where the secret lives for safe diagnostics. It must
	// never include secret material.
	Location() string
}

// Store coordinates the keyring-first storage policy and the nonsecret
// credential reference in the configuration file.
type Store struct {
	cfg     *config.Store
	keyring Backend
	file    Backend
	warn    io.Writer
}

// NewStore creates a Store bound to a configuration directory. The keyring is
// preferred; the unencrypted file is the warned fallback.
func NewStore(cfg *config.Store, warn io.Writer) *Store {
	return newStore(cfg, newKeyringBackend(), newFileBackend(cfg.Dir()), warn)
}

// NewStoreWithKeyring creates a Store with an explicit keyring backend and the
// default file fallback. It is the injection seam for tests and alternative OS
// keyring implementations.
func NewStoreWithKeyring(cfg *config.Store, keyringBackend Backend, warn io.Writer) *Store {
	return newStore(cfg, keyringBackend, newFileBackend(cfg.Dir()), warn)
}

func newStore(cfg *config.Store, keyringBackend, fileBackend Backend, warn io.Writer) *Store {
	return &Store{cfg: cfg, keyring: keyringBackend, file: fileBackend, warn: warn}
}

// Save stores a secret and records the used backend in the profile, all under
// the configuration lock so a concurrent Delete cannot interleave between the
// secret write and the reference write. It clears any stale credential in the
// other backend so a later switch cannot resurrect it.
func (s *Store) Save(ctx context.Context, profileName, apiURL, secret string) (string, error) {
	var backend string
	err := s.cfg.Update(ctx, func(cfg *config.Config) error {
		stored, err := s.storeSecret(profileName, apiURL, secret)
		if err != nil {
			return err
		}
		backend = stored
		profile := cfg.EnsureProfile(profileName)
		// An explicit login also binds the profile's nonsecret URL to the
		// credential it just stored.
		profile.APIURL = apiURL
		profile.SetCredentialFor(apiURL, stored)
		if stored == config.BackendKeyring {
			delete(cfg.Warnings, "unencrypted:"+profileName+":"+apiURL)
		}
		cfg.PutProfile(profileName, *profile)
		if cfg.DefaultProfile == "" {
			cfg.DefaultProfile = profileName
		}
		return nil
	})
	if err != nil {
		if backend != "" {
			// The secret was stored but the configuration write failed; remove
			// the now-orphaned secret rather than leaving it unreachable.
			_ = s.backendFor(backend).Delete(profileName, apiURL)
		}
		return "", err
	}
	if backend == config.BackendFile {
		s.warnFile(ctx, profileName, apiURL)
	}
	return backend, nil
}

func (s *Store) storeSecret(profileName, apiURL, secret string) (string, error) {
	err := s.keyring.Set(profileName, apiURL, secret)
	switch {
	case err == nil:
		// Remove any fallback copy left by an earlier unavailable keyring.
		if fileErr := s.file.Delete(profileName, apiURL); fileErr != nil && !errors.Is(fileErr, ErrNotStored) {
			return config.BackendKeyring, fmt.Errorf("remove stale fallback credential: %w", fileErr)
		}
		return config.BackendKeyring, nil
	case errors.Is(err, ErrKeyringUnavailable):
		if fileErr := s.file.Set(profileName, apiURL, secret); fileErr != nil {
			return "", fileErr
		}
		if keyringErr := s.keyring.Delete(profileName, apiURL); keyringErr != nil &&
			!errors.Is(keyringErr, ErrNotStored) && !errors.Is(keyringErr, ErrKeyringUnavailable) {
			return config.BackendFile, fmt.Errorf("remove stale keyring credential: %w", keyringErr)
		}
		return config.BackendFile, nil
	default:
		return "", fmt.Errorf("store credential in OS keyring: %w", err)
	}
}

// Load returns the stored secret for the profile/URL pair. A false stored
// result means no credential reference is recorded. A backend failure, or a
// recorded reference whose secret is absent, is returned as an error rather
// than reported as "not logged in".
func (s *Store) Load(ctx context.Context, profileName, apiURL string) (string, bool, error) {
	cfg, err := s.cfg.Load(ctx)
	if err != nil {
		return "", false, err
	}
	profile, ok := cfg.Profile(profileName)
	if !ok {
		return "", false, nil
	}
	ref, ok := profile.CredentialFor(apiURL)
	if !ok {
		return "", false, nil
	}
	backend := s.backendFor(ref.Backend)
	if backend == nil {
		return "", false, fmt.Errorf("unknown credential backend %q", ref.Backend)
	}
	if ref.Backend == config.BackendFile {
		s.warnFile(ctx, profileName, apiURL)
	}
	secret, err := backend.Get(profileName, apiURL)
	if errors.Is(err, ErrNotStored) {
		return "", false, fmt.Errorf("%w for profile %q", ErrCredentialMissing, profileName)
	}
	if err != nil {
		return "", false, err
	}
	return secret, true, nil
}

// Delete removes the credential from the recorded backend and best-effort from
// the other backend, then clears the configuration reference, all under the
// configuration lock. A real failure from either backend is returned so logout
// cannot silently leave a secret behind.
func (s *Store) Delete(ctx context.Context, profileName, apiURL string) error {
	return s.cfg.Update(ctx, func(cfg *config.Config) error {
		recorded := ""
		if profile, ok := cfg.Profile(profileName); ok {
			if ref, ok := profile.CredentialFor(apiURL); ok {
				recorded = ref.Backend
			}
		}

		var deleteErr error
		for _, backend := range []Backend{s.keyring, s.file} {
			err := backend.Delete(profileName, apiURL)
			if err == nil || errors.Is(err, ErrNotStored) {
				continue
			}
			if errors.Is(err, ErrKeyringUnavailable) && backend.Name() != recorded {
				// An unavailable non-recorded backend cannot hold the secret.
				continue
			}
			if deleteErr == nil {
				deleteErr = fmt.Errorf("remove credential from %s: %w", backend.Name(), err)
			}
		}
		if deleteErr != nil {
			return deleteErr
		}

		if profile, ok := cfg.Profile(profileName); ok {
			profile.DeleteCredentialFor(apiURL)
			cfg.PutProfile(profileName, profile)
			delete(cfg.Warnings, "unencrypted:"+profileName+":"+apiURL)
		}
		return nil
	})
}

func (s *Store) backendFor(name string) Backend {
	switch name {
	case config.BackendKeyring:
		return s.keyring
	case config.BackendFile:
		return s.file
	default:
		return nil
	}
}

func (s *Store) warnFile(ctx context.Context, profileName, apiURL string) {
	if s.warn == nil {
		return
	}
	writer := s.cfg.WarningWriter(ctx, "unencrypted:"+profileName+":"+apiURL, s.warn)
	_, _ = fmt.Fprintf(writer, "warning: credential stored unencrypted at %s; use an OS keyring to protect it at rest.\n", s.file.Location())
}
