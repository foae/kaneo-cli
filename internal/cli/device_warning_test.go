package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/foae/kaneo-cli/internal/auth"
)

func TestDeviceLoginDefaultDestinationWarning(t *testing.T) {
	env := newTestEnv(t)
	attempts := 0
	env.app.deviceLogin = func(context.Context, auth.DeviceOptions) (auth.DeviceToken, error) {
		attempts++
		if strings.Count(env.stderr.String(), "implicit Kaneo Cloud") != 1 {
			t.Fatalf("destination warning missing before device authorization: %s", env.stderr)
		}
		if attempts == 1 {
			return auth.DeviceToken{}, auth.ErrDeviceAccessDenied
		}
		return auth.DeviceToken{AccessToken: "synthetic-device-token", TokenType: "Bearer"}, nil
	}
	status, stdout, stderr := env.run("auth", "login")
	if status != 3 || stdout != "" || strings.Count(stderr, "implicit Kaneo Cloud") != 1 {
		t.Fatalf("denied login: status=%d stdout=%s stderr=%s", status, stdout, stderr)
	}
	status, _, stderr = env.run("auth", "login")
	if status != 0 || strings.Count(stderr, "implicit Kaneo Cloud") != 1 {
		t.Fatalf("successful retry: status=%d stderr=%s", status, stderr)
	}
	status, _, stderr = env.run("profile", "get")
	if status != 0 || strings.Contains(stderr, "implicit Kaneo Cloud") {
		t.Fatalf("offline inspection: status=%d stderr=%s", status, stderr)
	}
	env.app.deviceLogin = func(context.Context, auth.DeviceOptions) (auth.DeviceToken, error) {
		return auth.DeviceToken{AccessToken: "synthetic-device-token", TokenType: "Bearer"}, nil
	}
	status, _, stderr = env.run("auth", "login")
	if status != 0 || strings.Contains(stderr, "implicit Kaneo Cloud") {
		t.Fatalf("persisted destination: status=%d stderr=%s", status, stderr)
	}
}
