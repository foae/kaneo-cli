package cli

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/foae/kaneo-cli/internal/auth"
	"github.com/foae/kaneo-cli/internal/config"
)

func TestLocalProfileCommandsIgnoreMalformedRequestOverrides(t *testing.T) {
	env := newTestEnv(t)
	for _, name := range []string{"first", "second"} {
		if status, _, stderr := env.run("profile", "set", name, "--api-url", "https://"+name+".example/api", "--timeout", "20s"); status != 0 {
			t.Fatalf("create %q: status=%d stderr=%q", name, status, stderr)
		}
	}
	env.env["KANEO_API_URL"] = "://not-a-url"
	env.env["KANEO_TIMEOUT"] = "not-a-duration"

	status, stdout, stderr := env.run("profile", "list")
	if status != 0 {
		t.Fatalf("list: status=%d stderr=%q", status, stderr)
	}
	var profiles []profileView
	if err := json.Unmarshal([]byte(stdout), &profiles); err != nil {
		t.Fatalf("list output: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("listed profiles=%+v, want two", profiles)
	}

	if status, _, stderr = env.run("profile", "use", "second"); status != 0 {
		t.Fatalf("use: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr = env.run("profile", "set", "first", "--timeout", "45s"); status != 0 {
		t.Fatalf("set: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr = env.run("profile", "delete", "first", "--yes"); status != 0 {
		t.Fatalf("delete: status=%d stderr=%q", status, stderr)
	}

	cfg, err := config.NewStore(env.dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProfile != "second" {
		t.Fatalf("default profile=%q, want second", cfg.DefaultProfile)
	}
	if _, ok := cfg.Profile("first"); ok {
		t.Fatal("deleted profile remains in configuration")
	}
}

func TestProfileSetRejectsExplicitMalformedRequestSettings(t *testing.T) {
	env := newTestEnv(t)
	env.env["KANEO_API_URL"] = "://not-a-url"
	env.env["KANEO_TIMEOUT"] = "not-a-duration"

	if status, _, stderr := env.run("profile", "set", "bad-url", "--api-url", "not-a-url"); status != 2 {
		t.Fatalf("invalid API URL: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("profile", "set", "bad-timeout", "--timeout", "not-a-duration"); status != 2 {
		t.Fatalf("invalid timeout: status=%d stderr=%q", status, stderr)
	}

	cfg, err := config.NewStore(env.dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("invalid profile settings persisted: %+v", cfg.Profiles)
	}
}

func TestLocalProfileCommandsKeepExplicitMissingProfileValidation(t *testing.T) {
	env := newTestEnv(t)
	env.env["KANEO_API_URL"] = "://not-a-url"
	env.env["KANEO_TIMEOUT"] = "not-a-duration"

	if status, _, stderr := env.run("--profile", "missing", "profile", "list"); status != 2 {
		t.Fatalf("missing profile: status=%d stderr=%q", status, stderr)
	}
}

func TestLogoutIgnoresMalformedRequestOverridesAndKeepsOtherProfilesIsolated(t *testing.T) {
	env := newTestEnv(t)
	urls := []string{"https://one.example/api", "https://two.example/api"}
	store := config.NewStore(env.dir)
	if err := store.Update(context.Background(), func(cfg *config.Config) error {
		work := config.Profile{APIURL: urls[0]}
		for _, apiURL := range urls {
			work.SetCredentialFor(apiURL, config.BackendKeyring)
		}
		cfg.PutProfile("work", work)
		other := config.Profile{APIURL: urls[0]}
		other.SetCredentialFor(urls[0], config.BackendKeyring)
		cfg.PutProfile("other", other)
		cfg.DefaultProfile = "work"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, apiURL := range urls {
		if err := env.keyring.Set("work", apiURL, "work-secret"); err != nil {
			t.Fatal(err)
		}
	}
	if err := env.keyring.Set("other", urls[0], "other-secret"); err != nil {
		t.Fatal(err)
	}
	env.env["KANEO_API_URL"] = "://not-a-url"
	env.env["KANEO_TIMEOUT"] = "not-a-duration"

	if status, _, stderr := env.run("auth", "logout", "work", "--yes"); status != 0 {
		t.Fatalf("logout: status=%d stderr=%q", status, stderr)
	}
	for _, apiURL := range urls {
		if _, err := env.keyring.Get("work", apiURL); !errors.Is(err, auth.ErrNotStored) {
			t.Fatalf("work credential for %q remains: %v", apiURL, err)
		}
	}
	if secret, err := env.keyring.Get("other", urls[0]); err != nil || secret != "other-secret" {
		t.Fatalf("other profile credential=(%q, %v), want preserved credential", secret, err)
	}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	work, ok := cfg.Profile("work")
	if !ok {
		t.Fatal("work profile was removed")
	}
	if got := work.CredentialURLs(); len(got) != 0 {
		t.Fatalf("work credential references remain: %v", got)
	}
}
