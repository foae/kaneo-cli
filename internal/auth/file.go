package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/foae/kaneo-cli/internal/config"
)

const (
	credentialsFileName = "credentials.json"
	lockTimeout         = 5 * time.Second
	// maxCredentialsBytes bounds the credential file read.
	maxCredentialsBytes = 1 << 20
)

// fileBackend is the explicitly warned, unencrypted fallback store. It enforces
// owner-only access and atomic replacement, and refuses symlinks or foreign
// ownership.
type fileBackend struct {
	path string
}

type credentialsFile struct {
	Profiles map[string]map[string]string `json:"profiles"`
}

func newFileBackend(dir string) Backend {
	return &fileBackend{path: filepath.Join(dir, credentialsFileName)}
}

func (f *fileBackend) Name() string { return config.BackendFile }

func (f *fileBackend) Location() string { return f.path }

func (f *fileBackend) Get(profile, apiURL string) (string, error) {
	var secret string
	var found bool
	err := f.withLock(func() error {
		store, err := f.read()
		if err != nil {
			return err
		}
		secret, found = lookup(store, profile, apiURL)
		return nil
	})
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNotStored
	}
	return secret, nil
}

func (f *fileBackend) Set(profile, apiURL, secret string) error {
	return f.withLock(func() error {
		store, err := f.read()
		if err != nil {
			return err
		}
		if store.Profiles == nil {
			store.Profiles = make(map[string]map[string]string)
		}
		if store.Profiles[profile] == nil {
			store.Profiles[profile] = make(map[string]string)
		}
		store.Profiles[profile][apiURL] = secret
		return f.write(store)
	})
}

func (f *fileBackend) Delete(profile, apiURL string) error {
	var found bool
	err := f.withLock(func() error {
		store, err := f.read()
		if err != nil {
			return err
		}
		if _, ok := lookup(store, profile, apiURL); !ok {
			return nil
		}
		found = true
		delete(store.Profiles[profile], apiURL)
		if len(store.Profiles[profile]) == 0 {
			delete(store.Profiles, profile)
		}
		return f.write(store)
	})
	if err != nil {
		return err
	}
	if !found {
		return ErrNotStored
	}
	return nil
}

func lookup(store *credentialsFile, profile, apiURL string) (string, bool) {
	if store == nil || store.Profiles == nil {
		return "", false
	}
	byURL, ok := store.Profiles[profile]
	if !ok {
		return "", false
	}
	secret, ok := byURL[apiURL]
	return secret, ok
}

func (f *fileBackend) withLock(action func() error) error {
	dir := filepath.Dir(f.path)
	if info, err := os.Lstat(dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("credential directory %s is a symlink", dir)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect credential directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	if err := secureDir(dir); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	lock := flock.New(f.path + ".lock")
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("lock credentials: %w", err)
	}
	if !locked {
		return errors.New("timed out waiting for the credentials lock")
	}
	defer func() { _ = lock.Unlock() }()
	return action()
}

func (f *fileBackend) read() (*credentialsFile, error) {
	handle, err := openSecretFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &credentialsFile{}, nil
		}
		return nil, fmt.Errorf("open credential file: %w", err)
	}
	defer func() { _ = handle.Close() }()

	info, err := handle.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect credential file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("credential file %s is a symlink", f.path)
	}
	if err := checkOwner(info, f.path); err != nil {
		return nil, err
	}
	if err := checkPermissions(info, f.path); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(handle, maxCredentialsBytes))
	if err != nil {
		return nil, fmt.Errorf("read credential file: %w", err)
	}
	var store credentialsFile
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse credential file %s: %w", f.path, err)
	}
	return &store, nil
}

func (f *fileBackend) write(store *credentialsFile) error {
	if info, err := os.Lstat(f.path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("credential file %s is a symlink", f.path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect credential file: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	data = append(data, '\n')

	temp, err := os.CreateTemp(filepath.Dir(f.path), ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary credentials: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	// Restrict access before any secret byte is written so the file is never
	// briefly readable under an inherited (Windows) or default (Unix) mode.
	if err := secureSecretFile(tempName); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary credentials: %w", err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary credentials: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("close temporary credentials: %w", err)
	}
	if err := os.Rename(tempName, f.path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("replace credential file: %w", err)
	}
	config.SyncDir(filepath.Dir(f.path))
	return nil
}
