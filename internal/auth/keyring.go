package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/foae/kaneo-cli/internal/config"
)

const keyringService = "kaneo-cli"

type keyringProvider interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type osKeyring struct{}

func (osKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (osKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }

func (osKeyring) Delete(service, user string) error { return keyring.Delete(service, user) }

type keyringBackend struct {
	provider keyringProvider
}

func newKeyringBackend() Backend {
	return &keyringBackend{provider: osKeyring{}}
}

// keyringAccount binds the secret to both profile and normalized API URL so a
// URL change cannot transplant a credential.
func keyringAccount(profile, apiURL string) string {
	return profile + "\x1f" + apiURL
}

func (k *keyringBackend) Name() string { return config.BackendKeyring }

func (k *keyringBackend) Location() string { return "OS keyring" }

func (k *keyringBackend) Set(profile, apiURL, secret string) error {
	if err := k.provider.Set(keyringService, keyringAccount(profile, apiURL), secret); err != nil {
		return classifyKeyringError(err)
	}
	return nil
}

func (k *keyringBackend) Get(profile, apiURL string) (string, error) {
	secret, err := k.provider.Get(keyringService, keyringAccount(profile, apiURL))
	switch {
	case err == nil:
		return secret, nil
	case errors.Is(err, keyring.ErrNotFound):
		return "", ErrNotStored
	default:
		return "", classifyKeyringError(err)
	}
}

func (k *keyringBackend) Delete(profile, apiURL string) error {
	err := k.provider.Delete(keyringService, keyringAccount(profile, apiURL))
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return ErrNotStored
	default:
		return classifyKeyringError(err)
	}
}

// classifyKeyringError distinguishes an unreachable keyring from a genuine
// store failure. Only connection-level failures are treated as unavailable so
// an access-denied or corrupt store is never silently replaced by the
// unencrypted fallback. The markers are platform library error strings; native
// platform evidence is still required.
func classifyKeyringError(err error) error {
	if errors.Is(err, keyring.ErrUnsupportedPlatform) {
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"was not provided by any .service files",
		"failed to connect to socket",
		"cannot autolaunch",
		"no such file or directory",
		"connection refused",
		"the name is not activatable",
		"dbus-launch",
		"session bus",
		"couldn't determine address of session bus",
		"unsupported platform",
		"not supported",
	} {
		if strings.Contains(message, marker) {
			return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
		}
	}
	return err
}
