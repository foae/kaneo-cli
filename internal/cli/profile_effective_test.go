package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/foae/kaneo-cli/internal/config"
)

func TestFreshProfileInspectionIsOffline(t *testing.T) {
	env := newTestEnv(t)
	status, stdout, stderr := env.run("profile", "get")
	if status != 0 || stderr != "" {
		t.Fatalf("status=%d stderr=%s", status, stderr)
	}
	var view profileView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.APIURL != "https://cloud.kaneo.app/api" || view.Sources["api_url"] != "default" {
		t.Fatalf("view=%+v", view)
	}
	if _, err := os.Stat(env.dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspection created config: %v", err)
	}
	status, _, _ = env.run("profile", "get", "default")
	if status != 2 {
		t.Fatalf("explicit absent profile status=%d", status)
	}
}

func TestProfileGetReportsStaleDefaultSettings(t *testing.T) {
	env := newTestEnv(t)
	store := config.NewStore(env.dir)
	if err := store.Update(context.Background(), func(cfg *config.Config) error {
		cfg.DefaultProfile = "removed"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	status, stdout, stderr := env.run("profile", "get")
	if status != 0 || stderr != "" {
		t.Fatalf("status=%d stderr=%s", status, stderr)
	}
	var view profileView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.Name != "removed" || view.APIURL != config.DefaultAPIURL || view.Sources["profile"] != "default_profile" || view.Authenticated {
		t.Fatalf("view=%+v", view)
	}
}

func TestProfileGetEffectivePrecedence(t *testing.T) {
	env := newTestEnv(t)
	status, _, stderr := env.run("profile", "set", "work", "--api-url", "https://stored.example/api", "--timeout", "1m")
	if status != 0 {
		t.Fatal(stderr)
	}
	env.env["KANEO_PROFILE"] = "missing"
	env.env["KANEO_API_URL"] = "https://environment.example/custom"
	env.env["KANEO_TIMEOUT"] = "45s"
	status, stdout, stderr := env.run("profile", "get", "work", "--profile", "also-missing")
	if status != 0 {
		t.Fatal(stderr)
	}
	var view profileView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.APIURL != env.env["KANEO_API_URL"] || view.Timeout != "45s" || view.Sources["api_url"] != "environment" || view.Sources["profile"] != "argument" {
		t.Fatalf("view=%+v", view)
	}
	status, stdout, stderr = env.run("profile", "get", "work", "--api-url", "https://flag.example/prefix/", "--timeout", "90s")
	if status != 0 {
		t.Fatal(stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.APIURL != "https://flag.example/prefix" || view.Timeout != "1m30s" || view.Sources["api_url"] != "flag" || view.Sources["timeout"] != "flag" {
		t.Fatalf("view=%+v", view)
	}
	env.env["KANEO_API_URL"] = ""
	env.env["KANEO_TIMEOUT"] = ""
	status, stdout, stderr = env.run("profile", "get", "work")
	if status != 0 {
		t.Fatal(stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.APIURL != "https://stored.example/api" || view.Timeout != "1m0s" || view.Sources["api_url"] != "profile" {
		t.Fatalf("view=%+v", view)
	}
}

func TestDefaultDestinationWarningBeforeLoginPersistence(t *testing.T) {
	env := newTestEnv(t)
	env.setStdin("synthetic-api-key")
	status, _, stderr := env.run("auth", "login", "--api-key-file", "-")
	if status != 0 || strings.Count(stderr, "implicit Kaneo Cloud") != 1 {
		t.Fatalf("status=%d stderr=%s", status, stderr)
	}
	env.setStdin("synthetic-api-key")
	status, _, stderr = env.run("auth", "login", "--api-key-file", "-")
	if status != 0 || strings.Contains(stderr, "implicit Kaneo Cloud") {
		t.Fatalf("persisted destination status=%d stderr=%s", status, stderr)
	}
	explicit := newTestEnv(t)
	explicit.setStdin("synthetic-api-key")
	status, _, stderr = explicit.run("auth", "login", "--api-key-file", "-", "--api-url", "https://cloud.kaneo.app/api")
	if status != 0 || strings.Contains(stderr, "implicit Kaneo Cloud") {
		t.Fatalf("explicit destination status=%d stderr=%s", status, stderr)
	}
}

func TestProfileGetDoesNotReportAnotherDestinationsCredential(t *testing.T) {
	env := newTestEnv(t)
	env.setStdin("synthetic-api-key")
	status, _, stderr := env.run("auth", "login", "--api-key-file", "-", "--api-url", "https://stored.example/api")
	if status != 0 {
		t.Fatal(stderr)
	}
	status, stdout, stderr := env.run("profile", "get", "--api-url", "https://override.example/api")
	if status != 0 {
		t.Fatal(stderr)
	}
	var view profileView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.Authenticated || view.Storage != "" || view.APIURL != "https://override.example/api" {
		t.Fatalf("view=%+v", view)
	}
}

func TestDefaultDestinationWarningBeforeCanceledPublicRequest(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status := executeContext(ctx, env.app, []string{"instance", "get-status"})
	if status != 130 || env.stdout.Len() != 0 || strings.Count(env.stderr.String(), "implicit Kaneo Cloud") != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, env.stdout, env.stderr)
	}
}
