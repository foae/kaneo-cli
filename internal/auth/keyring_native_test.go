package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
)

func TestNativeKeyringCredentialLifecycle(t *testing.T) {
	if os.Getenv("KANEO_NATIVE_KEYRING_TEST") != "1" {
		t.Skip("set KANEO_NATIVE_KEYRING_TEST=1 to exercise the native OS keyring")
	}

	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		t.Fatal("generate unique native keyring test identifier")
	}
	id := hex.EncodeToString(identifier)
	profile := "native-keyring-" + id
	apiURL := "http://127.0.0.1/" + id
	secret := "synthetic-native-keyring-secret-" + id
	backend := newKeyringBackend()

	t.Cleanup(func() {
		if err := backend.Delete(profile, apiURL); err != nil && !errors.Is(err, ErrNotStored) {
			t.Errorf("native keyring cleanup: %v", err)
		}
	})

	if err := backend.Delete(profile, apiURL); err != nil && !errors.Is(err, ErrNotStored) {
		t.Fatalf("clear native keyring test credential: %v", err)
	}
	if err := backend.Set(profile, apiURL, secret); err != nil {
		t.Fatalf("store native keyring test credential: %v", err)
	}

	stored, err := backend.Get(profile, apiURL)
	if err != nil {
		t.Fatalf("retrieve native keyring test credential: %v", err)
	}
	if stored != secret {
		t.Fatal("native keyring returned a different credential")
	}

	if err := backend.Delete(profile, apiURL); err != nil {
		t.Fatalf("delete native keyring test credential: %v", err)
	}
	if _, err := backend.Get(profile, apiURL); !errors.Is(err, ErrNotStored) {
		t.Fatalf("retrieve deleted native keyring test credential: %v, want ErrNotStored", err)
	}
}
