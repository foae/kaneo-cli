package config

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestResolvePrecedence(t *testing.T) {
	cfg := &Config{
		DefaultProfile: "work",
		Profiles: map[string]Profile{
			"work":  {APIURL: "https://work.example.com/api", Timeout: "45s"},
			"other": {APIURL: "https://other.example.com/api"},
		},
	}

	tests := []struct {
		name        string
		resolver    Resolver
		wantProfile string
		wantURL     string
		wantTimeout time.Duration
	}{
		{
			name:        "flag wins",
			resolver:    Resolver{Config: cfg, FlagProfile: "work", EnvProfile: "other", FlagAPIURL: "https://flag.example.com/api", EnvAPIURL: "https://env.example.com/api", FlagTimeout: "5s", EnvTimeout: "6s"},
			wantProfile: "work",
			wantURL:     "https://flag.example.com/api",
			wantTimeout: 5 * time.Second,
		},
		{
			name:        "environment beats profile",
			resolver:    Resolver{Config: cfg, EnvProfile: "other", EnvAPIURL: "https://env.example.com/api", EnvTimeout: "9s"},
			wantProfile: "other",
			wantURL:     "https://env.example.com/api",
			wantTimeout: 9 * time.Second,
		},
		{
			name:        "default profile and its settings",
			resolver:    Resolver{Config: cfg},
			wantProfile: "work",
			wantURL:     "https://work.example.com/api",
			wantTimeout: 45 * time.Second,
		},
		{
			name:        "documented defaults when nothing set",
			resolver:    Resolver{Config: &Config{}},
			wantProfile: DefaultProfileName,
			wantURL:     DefaultAPIURL,
			wantTimeout: DefaultTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.resolver.Resolve()
			if err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}
			if got.ProfileName != tt.wantProfile {
				t.Fatalf("profile = %q, want %q", got.ProfileName, tt.wantProfile)
			}
			if got.APIURL != tt.wantURL {
				t.Fatalf("api url = %q, want %q", got.APIURL, tt.wantURL)
			}
			if got.Timeout != tt.wantTimeout {
				t.Fatalf("timeout = %v, want %v", got.Timeout, tt.wantTimeout)
			}
		})
	}
}

func TestResolveRejectsInvalidInput(t *testing.T) {
	if _, err := (Resolver{Config: &Config{}, FlagAPIURL: "not-a-url"}).Resolve(); err == nil {
		t.Fatal("Resolve() accepted an invalid URL")
	}
	if _, err := (Resolver{Config: &Config{}, FlagTimeout: "soon"}).Resolve(); err == nil {
		t.Fatal("Resolve() accepted an invalid timeout")
	}
	if _, err := (Resolver{Config: &Config{}, FlagTimeout: "-5s"}).Resolve(); err == nil {
		t.Fatal("Resolve() accepted a negative timeout")
	}
}

func TestLoadMissingDoesNotCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "kaneo-cli")
	store := NewStore(dir)

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("Load() = %+v, want empty", cfg)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Load() created %s", dir)
	}
}

func TestUpdateWritesSecurely(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kaneo-cli")
	store := NewStore(dir)

	err := store.Update(context.Background(), func(cfg *Config) error {
		cfg.PutProfile("default", Profile{APIURL: "https://example.com/api"})
		cfg.DefaultProfile = "default"
		return nil
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("config mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); runtime.GOOS != "windows" && got != 0o700 {
		t.Fatalf("dir mode = %o, want 700", got)
	}
}

func TestConcurrentUpdatesDoNotLoseProfiles(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "kaneo-cli"))

	const workers = 8
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			name := "profile-" + string(rune('a'+index))
			errs <- store.Update(context.Background(), func(cfg *Config) error {
				cfg.PutProfile(name, Profile{APIURL: "https://example.com/api"})
				return nil
			})
		}(i)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Update() error: %v", err)
		}
	}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != workers {
		t.Fatalf("profiles = %d, want %d: %+v", len(cfg.Profiles), workers, cfg.Profiles)
	}
}

func TestUpdateRefusesSymlinkTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kaneo-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim.json")
	if err := os.WriteFile(victim, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, fileName)); err != nil {
		t.Fatal(err)
	}

	store := NewStore(dir)
	err := store.Update(context.Background(), func(cfg *Config) error {
		cfg.PutProfile("x", Profile{})
		return nil
	})
	if err == nil {
		t.Fatal("Update() followed a symlinked config file")
	}
}

func TestDeleteProfileClearsDefault(t *testing.T) {
	cfg := &Config{DefaultProfile: "a", Profiles: map[string]Profile{"a": {}}}
	if !cfg.DeleteProfile("a") {
		t.Fatal("DeleteProfile() = false")
	}
	if cfg.DefaultProfile != "" {
		t.Fatalf("default = %q, want empty", cfg.DefaultProfile)
	}
	if _, ok := cfg.Profile("a"); ok {
		t.Fatal("profile still present")
	}
}

func TestURLKeyNormalizes(t *testing.T) {
	got, err := URLKey("HTTPS://Example.COM:443/api/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/api" {
		t.Fatalf("URLKey() = %q", got)
	}
}
