// Package config owns local profiles: the nonsecret configuration file, its
// atomic updates and locking, and precedence resolution.
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/gofrs/flock"

	"github.com/foae/kaneo-cli/internal/client"
)

const (
	dirName  = "kaneo-cli"
	fileName = "config.json"
	lockName = "config.lock"

	dirMode  = 0o700
	fileMode = 0o600

	// lockWait bounds how long an update waits for a concurrent writer before
	// failing, so a stuck holder cannot hang the CLI indefinitely.
	lockWait = 5 * time.Second
)

// Credential backends recorded in the nonsecret configuration.
const (
	BackendKeyring = "keyring"
	BackendFile    = "file"
)

// Config is the nonsecret configuration file. It never contains a token.
type Config struct {
	DefaultProfile string             `json:"default_profile,omitempty"`
	Profiles       map[string]Profile `json:"profiles,omitempty"`
}

// Profile is one named local profile bound to a normalized API base URL.
type Profile struct {
	APIURL  string `json:"api_url,omitempty"`
	Timeout string `json:"timeout,omitempty"`
	// Credentials records, per normalized API URL, which backend holds the
	// secret. It is a reference map, never the secret itself.
	Credentials map[string]CredentialRef `json:"credentials,omitempty"`
}

// CredentialRef records the active backend for one profile/URL pair.
type CredentialRef struct {
	Backend string `json:"backend"`
}

// Profile returns a copy of the named profile.
func (c *Config) Profile(name string) (Profile, bool) {
	profile, ok := c.Profiles[name]
	return profile, ok
}

// EnsureProfile returns a mutable copy of the profile, or a zero value when it
// is absent. Callers persist the result with PutProfile.
func (c *Config) EnsureProfile(name string) *Profile {
	if c.Profiles == nil {
		c.Profiles = make(map[string]Profile)
	}
	profile := c.Profiles[name]
	return &profile
}

// PutProfile stores a profile.
func (c *Config) PutProfile(name string, profile Profile) {
	if c.Profiles == nil {
		c.Profiles = make(map[string]Profile)
	}
	c.Profiles[name] = profile
}

// DeleteProfile removes a profile and reports whether it existed.
func (c *Config) DeleteProfile(name string) bool {
	if _, ok := c.Profiles[name]; !ok {
		return false
	}
	delete(c.Profiles, name)
	if c.DefaultProfile == name {
		c.DefaultProfile = ""
	}
	return true
}

// Names returns the sorted profile names.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CredentialFor returns the recorded backend for a normalized API URL.
func (p Profile) CredentialFor(apiURL string) (CredentialRef, bool) {
	ref, ok := p.Credentials[apiURL]
	return ref, ok
}

// SetCredentialFor records the backend for a normalized API URL.
func (p *Profile) SetCredentialFor(apiURL, backend string) {
	if p.Credentials == nil {
		p.Credentials = make(map[string]CredentialRef)
	}
	p.Credentials[apiURL] = CredentialRef{Backend: backend}
}

// DeleteCredentialFor removes the backend record for a URL.
func (p *Profile) DeleteCredentialFor(apiURL string) {
	delete(p.Credentials, apiURL)
}

// CredentialURLs returns the sorted URLs with a recorded credential.
func (p Profile) CredentialURLs() []string {
	urls := make([]string, 0, len(p.Credentials))
	for apiURL := range p.Credentials {
		urls = append(urls, apiURL)
	}
	sort.Strings(urls)
	return urls
}

// Store reads and writes the configuration file under one directory.
type Store struct {
	dir string
}

// DefaultDir returns the platform configuration directory for this CLI. It
// performs no filesystem changes.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, dirName), nil
}

// NewStore creates a Store rooted at dir.
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Dir returns the configuration directory.
func (s *Store) Dir() string { return s.dir }

// Path returns the configuration file path.
func (s *Store) Path() string { return filepath.Join(s.dir, fileName) }

// Load reads the configuration, returning an empty configuration when no file
// exists. It never creates the directory or file.
func (s *Store) Load(_ context.Context) (*Config, error) {
	return s.read()
}

func (s *Store) read() (*Config, error) {
	data, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse configuration %s: %w", s.Path(), err)
	}
	return &cfg, nil
}

// Update locks the store, reads the latest configuration, applies mutate and
// atomically replaces the file. The lock is released on every path.
func (s *Store) Update(ctx context.Context, mutate func(*Config) error) error {
	if err := s.prepareDir(); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(s.dir, lockName))
	lockCtx, cancel := context.WithTimeout(ctx, lockWait)
	defer cancel()
	locked, err := lock.TryLockContext(lockCtx, 50*time.Millisecond)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("timed out waiting for the configuration lock")
		}
		return fmt.Errorf("lock configuration: %w", err)
	}
	if !locked {
		return errors.New("timed out waiting for the configuration lock")
	}
	defer func() { _ = lock.Unlock() }()

	cfg, err := s.read()
	if err != nil {
		return err
	}
	if err := mutate(cfg); err != nil {
		return err
	}
	return s.write(cfg)
}

func (s *Store) prepareDir() error {
	if info, err := os.Lstat(s.dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("configuration directory %s is a symlink", s.dir)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect configuration directory: %w", err)
	}
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(s.dir, dirMode); err != nil {
			return fmt.Errorf("secure configuration directory: %w", err)
		}
	}
	return nil
}

func (s *Store) write(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	data = append(data, '\n')

	if info, err := os.Lstat(s.Path()); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("configuration file %s is a symlink", s.Path())
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect configuration file: %w", err)
	}

	temp, err := os.CreateTemp(s.dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	if runtime.GOOS != "windows" {
		if err := temp.Chmod(fileMode); err != nil {
			cleanup()
			return fmt.Errorf("secure temporary configuration: %w", err)
		}
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary configuration: %w", err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary configuration: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("close temporary configuration: %w", err)
	}
	if err := os.Rename(tempName, s.Path()); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("replace configuration: %w", err)
	}
	SyncDir(s.dir)
	return nil
}

// SyncDir best-effort flushes a directory entry so a rename survives a crash.
// Windows cannot open a directory for sync, so errors are intentionally ignored.
func SyncDir(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	defer func() { _ = handle.Close() }()
	_ = handle.Sync()
}

// URLKey validates and normalizes an API URL for use as a credential key.
func URLKey(raw string) (string, error) {
	base, err := client.ParseBaseURL(raw)
	if err != nil {
		return "", err
	}
	return base.String(), nil
}
