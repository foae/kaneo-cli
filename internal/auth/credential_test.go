package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/foae/kaneo-cli/internal/config"
)

type memKeyring struct {
	mu     sync.Mutex
	values map[string]string
	err    error
}

func newMemKeyring() *memKeyring {
	return &memKeyring{values: map[string]string{}}
}

func (m *memKeyring) key(service, user string) string { return service + "\x00" + user }

func (m *memKeyring) Set(service, user, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.values[m.key(service, user)] = password
	return nil
}

func (m *memKeyring) Get(service, user string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	value, ok := m.values[m.key(service, user)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (m *memKeyring) Delete(service, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	key := m.key(service, user)
	if _, ok := m.values[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(m.values, key)
	return nil
}

func newTestStore(t *testing.T, keyringBackend Backend, warn io.Writer) (*Store, *config.Store) {
	t.Helper()
	cfg := config.NewStore(filepath.Join(t.TempDir(), "kaneo-cli"))
	return newStore(cfg, keyringBackend, newFileBackend(cfg.Dir()), warn), cfg
}

func TestCredentialLifecycleWithKeyring(t *testing.T) {
	ctx := context.Background()
	keyringBackend := &keyringBackend{provider: newMemKeyring()}
	var warn bytes.Buffer
	store, cfg := newTestStore(t, keyringBackend, &warn)

	backend, err := store.Save(ctx, "work", "https://example.com/api", "synthetic-secret")
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if backend != config.BackendKeyring {
		t.Fatalf("backend = %q, want keyring", backend)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning: %q", warn.String())
	}

	stored, err := cfg.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	profile, _ := stored.Profile("work")
	if ref, ok := profile.CredentialFor("https://example.com/api"); !ok || ref.Backend != config.BackendKeyring {
		t.Fatalf("config ref = %+v", ref)
	}

	secret, ok, err := store.Load(ctx, "work", "https://example.com/api")
	if err != nil || !ok || secret != "synthetic-secret" {
		t.Fatalf("Load() = %q, %v, %v", secret, ok, err)
	}
	if _, ok, _ := store.Load(ctx, "work", "https://other.example.com/api"); ok {
		t.Fatal("credential was transplanted to another URL")
	}

	if err := store.Delete(ctx, "work", "https://example.com/api"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if _, ok, _ := store.Load(ctx, "work", "https://example.com/api"); ok {
		t.Fatal("credential survived Delete()")
	}
}

func TestKeyringUnavailableFallsBackWithWarning(t *testing.T) {
	ctx := context.Background()
	unavailable := &keyringBackend{provider: &memKeyring{err: fmt.Errorf("%w: simulated", ErrKeyringUnavailable)}}
	var warn bytes.Buffer
	store, cfg := newTestStore(t, unavailable, &warn)

	backend, err := store.Save(ctx, "headless", "https://example.com/api", "synthetic-secret")
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if backend != config.BackendFile {
		t.Fatalf("backend = %q, want file", backend)
	}
	if !bytes.Contains(warn.Bytes(), []byte("unencrypted")) {
		t.Fatalf("warning missing: %q", warn.String())
	}

	stored, _ := cfg.Load(ctx)
	profile, _ := stored.Profile("headless")
	if ref, _ := profile.CredentialFor("https://example.com/api"); ref.Backend != config.BackendFile {
		t.Fatalf("ref = %+v", ref)
	}
	secret, ok, err := store.Load(ctx, "headless", "https://example.com/api")
	if err != nil || !ok || secret != "synthetic-secret" {
		t.Fatalf("Load() = %q, %v, %v", secret, ok, err)
	}

	info, err := os.Stat(filepath.Join(cfg.Dir(), credentialsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("credential file mode = %o, want 600", mode)
	}
}

func TestKeyringDeniedIsNotSilentlyDowngraded(t *testing.T) {
	ctx := context.Background()
	denied := &keyringBackend{provider: &memKeyring{err: errors.New("AccessDenied: keyring locked")}}
	var warn bytes.Buffer
	store, cfg := newTestStore(t, denied, &warn)

	_, err := store.Save(ctx, "work", "https://example.com/api", "synthetic-secret")
	if err == nil {
		t.Fatal("Save() succeeded despite an access-denied keyring")
	}
	if _, statErr := os.Stat(filepath.Join(cfg.Dir(), credentialsFileName)); !os.IsNotExist(statErr) {
		t.Fatal("Save() wrote a plaintext fallback after access denied")
	}
}

func TestLoadSurfacesBackendFailure(t *testing.T) {
	ctx := context.Background()
	available := &keyringBackend{provider: newMemKeyring()}
	store, _ := newTestStore(t, available, io.Discard)

	if _, err := store.Save(ctx, "work", "https://example.com/api", "synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	// The backend becomes unreachable after the reference was recorded.
	available.provider = &memKeyring{err: errors.New("AccessDenied: keyring locked")}

	_, _, err := store.Load(ctx, "work", "https://example.com/api")
	if err == nil {
		t.Fatal("Load() hid a backend failure as 'not logged in'")
	}
}

func TestBackendMigrationClearsFallback(t *testing.T) {
	ctx := context.Background()
	unavailable := &keyringBackend{provider: &memKeyring{err: fmt.Errorf("%w: simulated", ErrKeyringUnavailable)}}
	store, cfg := newTestStore(t, unavailable, io.Discard)
	if _, err := store.Save(ctx, "work", "https://example.com/api", "old"); err != nil {
		t.Fatal(err)
	}
	credentialPath := filepath.Join(cfg.Dir(), credentialsFileName)
	if _, err := os.Stat(credentialPath); err != nil {
		t.Fatalf("fallback file missing: %v", err)
	}

	// The keyring becomes available and the user logs in again.
	store.keyring = &keyringBackend{provider: newMemKeyring()}
	backend, err := store.Save(ctx, "work", "https://example.com/api", "new")
	if err != nil {
		t.Fatal(err)
	}
	if backend != config.BackendKeyring {
		t.Fatalf("backend = %q, want keyring", backend)
	}
	if _, err := store.file.Get("work", "https://example.com/api"); !errors.Is(err, ErrNotStored) {
		t.Fatalf("stale plaintext fallback survived migration: %v", err)
	}
	if _, statErr := os.Stat(credentialPath); statErr != nil && !os.IsNotExist(statErr) {
		t.Fatalf("stat credential file: %v", statErr)
	}
	secret, ok, err := store.Load(ctx, "work", "https://example.com/api")
	if err != nil || !ok || secret != "new" {
		t.Fatalf("Load() = %q, %v, %v", secret, ok, err)
	}
}

func TestFileBackendConcurrentWrites(t *testing.T) {
	backend := newFileBackend(t.TempDir())

	const workers = 8
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			profile := fmt.Sprintf("p%d", index)
			errs <- backend.Set(profile, "https://example.com/api", "secret")
		}(i)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Set() error: %v", err)
		}
	}
	for i := 0; i < workers; i++ {
		profile := fmt.Sprintf("p%d", i)
		if _, err := backend.Get(profile, "https://example.com/api"); err != nil {
			t.Fatalf("Get(%s) error: %v", profile, err)
		}
	}
}

func TestFileBackendRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	backend := newFileBackend(dir)
	victim := filepath.Join(t.TempDir(), "victim.json")
	if err := os.WriteFile(victim, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, credentialsFileName)); err != nil {
		t.Fatal(err)
	}
	if err := backend.Set("work", "https://example.com/api", "secret"); err == nil {
		t.Fatal("Set() followed a symlinked credential file")
	}
	if _, err := backend.Get("work", "https://example.com/api"); err == nil {
		t.Fatal("Get() followed a symlinked credential file")
	}
}

func TestClassifyKeyringError(t *testing.T) {
	tests := []struct {
		message     string
		unavailable bool
	}{
		{message: "The name org.freedesktop.secrets was not provided by any .service files", unavailable: true},
		{message: `exec: "dbus-launch": executable file not found in $PATH`, unavailable: true},
		{message: "dbus: couldn't determine address of session bus", unavailable: true},
		{message: "org.freedesktop.DBus.Error.AccessDenied: not permitted", unavailable: false},
	}
	for _, tt := range tests {
		if got := errors.Is(classifyKeyringError(errors.New(tt.message)), ErrKeyringUnavailable); got != tt.unavailable {
			t.Fatalf("classify(%q) unavailable = %v, want %v", tt.message, got, tt.unavailable)
		}
	}
	if !errors.Is(classifyKeyringError(keyring.ErrUnsupportedPlatform), ErrKeyringUnavailable) {
		t.Fatal("unsupported platform not classified as unavailable")
	}
}

func TestLoadWarnsWhenUsingFallback(t *testing.T) {
	ctx := context.Background()
	unavailable := &keyringBackend{provider: &memKeyring{err: fmt.Errorf("%w: simulated", ErrKeyringUnavailable)}}
	var warn bytes.Buffer
	store, _ := newTestStore(t, unavailable, &warn)
	if _, err := store.Save(ctx, "work", "https://example.com/api", "synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	warn.Reset()

	if _, ok, err := store.Load(ctx, "work", "https://example.com/api"); err != nil || !ok {
		t.Fatalf("Load() = %v, %v", ok, err)
	}
	if !bytes.Contains(warn.Bytes(), []byte("unencrypted")) {
		t.Fatalf("no warning when using fallback: %q", warn.String())
	}
}

func TestLoadReportsMissingSecret(t *testing.T) {
	ctx := context.Background()
	provider := newMemKeyring()
	store, _ := newTestStore(t, &keyringBackend{provider: provider}, io.Discard)
	if _, err := store.Save(ctx, "work", "https://example.com/api", "synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	// The secret disappears out from under the recorded reference.
	if err := provider.Delete(keyringService, keyringAccount("work", "https://example.com/api")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(ctx, "work", "https://example.com/api"); !errors.Is(err, ErrCredentialMissing) {
		t.Fatalf("Load() error = %v, want ErrCredentialMissing", err)
	}
}

func TestFileBackendRefusesLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission policy")
	}
	dir := t.TempDir()
	backend := newFileBackend(dir)
	if err := backend.Set("work", "https://example.com/api", "synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, credentialsFileName), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get("work", "https://example.com/api"); err == nil {
		t.Fatal("Get() accepted a group/world-readable credential file")
	}
}
