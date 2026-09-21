package config

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// WarningWriter remembers successfully displayed warnings in nonsecret config.
// If persistence fails, it still emits the warning rather than hiding a risk.
func (s *Store) WarningWriter(ctx context.Context, key string, out io.Writer) io.Writer {
	return &warningWriter{store: s, ctx: ctx, key: key, out: out}
}

type warningWriter struct {
	store *Store
	ctx   context.Context
	key   string
	out   io.Writer
}

func (w *warningWriter) Write(p []byte) (int, error) {
	if w.out == nil {
		return len(p), nil
	}
	cfg, err := w.store.Load(w.ctx)
	if err == nil && cfg.Warnings[w.key] {
		return len(p), nil
	}
	if err := w.store.prepareDir(); err != nil {
		return w.emit(p)
	}
	// Serialize warning delivery separately: a blocked output stream must not
	// hold the configuration lock needed by logout and profile repair.
	lock := flock.New(filepath.Join(w.store.dir, "warnings.lock"))
	ctx, cancel := context.WithTimeout(w.ctx, lockWait)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil || !locked {
		return w.emit(p)
	}
	defer func() { _ = lock.Unlock() }()
	cfg, err = w.store.Load(w.ctx)
	if err == nil && cfg.Warnings[w.key] {
		return len(p), nil
	}
	n, err := w.emit(p)
	if err != nil {
		return n, err
	}
	_ = w.store.Update(w.ctx, func(cfg *Config) error {
		if cfg.Warnings == nil {
			cfg.Warnings = make(map[string]bool)
		}
		cfg.Warnings[w.key] = true
		return nil
	})
	return len(p), nil
}

func (w *warningWriter) emit(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

// ResetWarnings forgets destination warnings for a removed or logged-out profile.
func (c *Config) ResetWarnings(profile string) {
	for key := range c.Warnings {
		for _, kind := range []string{"plain-http:", "unencrypted:"} {
			destination, ok := strings.CutPrefix(key, kind+profile+":")
			// Profile names cannot contain slashes. Requiring the URL scheme
			// boundary prevents a colon-containing name from matching a prefix.
			if ok && (strings.HasPrefix(destination, "http://") || strings.HasPrefix(destination, "https://")) {
				delete(c.Warnings, key)
			}
		}
	}
}
