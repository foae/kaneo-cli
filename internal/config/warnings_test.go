package config

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConcurrentWarningWritersEmitOnce(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			writer := NewStore(dir).WarningWriter(context.Background(), "risk", &out)
			if _, err := io.WriteString(writer, "warning\n"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if out.String() != "warning\n" {
		t.Fatalf("warnings=%q", out.String())
	}
}

type failedWarningOutput struct{}

func (failedWarningOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestFailedWarningOutputDoesNotSuppressRetry(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := io.WriteString(store.WarningWriter(context.Background(), "risk", failedWarningOutput{}), "warning\n"); err == nil {
		t.Fatal("failed output was accepted")
	}
	var out bytes.Buffer
	if _, err := io.WriteString(store.WarningWriter(context.Background(), "risk", &out), "warning\n"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "warning\n" {
		t.Fatalf("retry warning=%q", out.String())
	}
}

func TestUnwritableWarningStateStillWarns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := io.WriteString(NewStore(path).WarningWriter(context.Background(), "risk", &out), "warning\n"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "warning\n" {
		t.Fatalf("fallback warning=%q", out.String())
	}
}

type configUpdatingOutput struct {
	store *Store
}

func (w configUpdatingOutput) Write(p []byte) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := w.store.Update(ctx, func(cfg *Config) error {
		cfg.DefaultProfile = "repaired"
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func TestWarningOutputDoesNotHoldConfigLock(t *testing.T) {
	store := NewStore(t.TempDir())
	writer := store.WarningWriter(context.Background(), "risk", configUpdatingOutput{store: store})
	if _, err := io.WriteString(writer, "warning\n"); err != nil {
		t.Fatalf("output blocked configuration repair: %v", err)
	}
	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProfile != "repaired" {
		t.Fatal("warning persistence lost concurrent configuration repair")
	}
}

type shortWarningOutput struct{}

func (shortWarningOutput) Write([]byte) (int, error) { return 1, nil }

func TestShortWarningWriteReportsCountAndRetries(t *testing.T) {
	store := NewStore(t.TempDir())
	n, err := io.WriteString(store.WarningWriter(context.Background(), "risk", shortWarningOutput{}), "warning\n")
	if n != 1 || err != io.ErrShortWrite {
		t.Fatalf("Write() = %d, %v", n, err)
	}
	var out bytes.Buffer
	if _, err := io.WriteString(store.WarningWriter(context.Background(), "risk", &out), "warning\n"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "warning\n" {
		t.Fatalf("short write suppressed retry: %q", out.String())
	}
}

func TestResetWarningsPreservesOtherProfiles(t *testing.T) {
	store := NewStore(t.TempDir())
	for _, profile := range []string{"work", "work:other"} {
		for _, kind := range []string{"plain-http:", "unencrypted:"} {
			if _, err := io.WriteString(store.WarningWriter(context.Background(), kind+profile+":http://example.com/api", io.Discard), "warning\n"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := store.Update(context.Background(), func(cfg *Config) error {
		cfg.ResetWarnings("work")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"work", "work:other"} {
		for _, kind := range []string{"plain-http:", "unencrypted:"} {
			var out bytes.Buffer
			if _, err := io.WriteString(store.WarningWriter(context.Background(), kind+profile+":http://example.com/api", &out), "warning\n"); err != nil {
				t.Fatal(err)
			}
			if (out.Len() != 0) != (profile == "work") {
				t.Fatalf("incorrect warning lifecycle for %q: %q", profile, out.String())
			}
		}
	}
}
