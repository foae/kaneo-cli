package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/foae/kaneo-cli/internal/auth"
	"github.com/foae/kaneo-cli/internal/client"
	"github.com/foae/kaneo-cli/internal/config"
)

// session is the once-per-invocation resolution of profile, effective settings
// and credential. The credential is loaded lazily so public and local-profile
// commands never touch the keyring or the fallback file.
type session struct {
	app      *app
	store    *config.Store
	cfg      *config.Config
	resolved config.Resolved
	creds    *auth.Store
	token    string
	loaded   bool
}

func (a *app) session(ctx context.Context) (*session, error) {
	dir, err := a.configDir()
	if err != nil {
		return nil, err
	}
	store := config.NewStore(dir)
	cfg, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := a.resolver(cfg).Resolve()
	if err != nil {
		return nil, &usageError{err: err}
	}
	return &session{
		app:      a,
		store:    store,
		cfg:      cfg,
		resolved: resolved,
		creds:    a.credentialStore(store, a.streams.errOut),
	}, nil
}

func (a *app) resolver(cfg *config.Config) config.Resolver {
	return config.Resolver{
		Config:      cfg,
		FlagProfile: a.flags.profile,
		EnvProfile:  a.env("KANEO_PROFILE"),
		FlagAPIURL:  a.flags.apiURL,
		EnvAPIURL:   a.env("KANEO_API_URL"),
		FlagTimeout: a.flags.timeout,
		EnvTimeout:  a.env("KANEO_TIMEOUT"),
	}
}

func (a *app) env(key string) string {
	if a.getenv == nil {
		return ""
	}
	return a.getenv(key)
}

// loadToken resolves the credential once: the invocation-only KANEO_TOKEN
// override, then the stored credential for the effective profile and URL. A
// backend error is surfaced rather than treated as unauthenticated.
func (s *session) loadToken(ctx context.Context) error {
	if s.loaded {
		return nil
	}
	s.loaded = true
	if override := s.app.env("KANEO_TOKEN"); override != "" {
		s.token = override
		return nil
	}
	secret, ok, err := s.creds.Load(ctx, s.resolved.ProfileName, s.resolved.APIURL)
	if err != nil {
		return fmt.Errorf("load stored credential: %w", err)
	}
	if ok {
		s.token = secret
	}
	return nil
}

func (s *session) newClient(ctx context.Context) (*client.Client, error) {
	if err := s.loadToken(ctx); err != nil {
		return nil, &processError{err: err}
	}
	return s.clientWithToken(s.token)
}

// newUnauthenticatedClient is used for public operations and before a login,
// when no credential is valid yet.
func (s *session) newUnauthenticatedClient() (*client.Client, error) {
	return s.clientWithToken("")
}

func (s *session) clientWithToken(token string) (*client.Client, error) {
	created, err := client.New(client.Options{
		BaseURL:    s.resolved.Base,
		Token:      token,
		Timeout:    s.resolved.Timeout,
		WarnWriter: s.app.streams.errOut,
		UserAgent:  "kaneo-cli/" + s.app.info.Version,
	})
	if err != nil {
		return nil, &processError{err: err}
	}
	return created, nil
}

// writeJSONStream copies a successful API response to stdout unchanged, adding
// a trailing newline only when the body lacks one. No-content responses leave
// stdout empty and JSON null stays "null". A 2xx body that is not JSON is
// rejected rather than passed through as a false success.
func writeJSONStream(out io.Writer, resp *client.Response) error {
	defer func() { _ = resp.Body.Close() }()
	if resp.Empty() {
		return nil
	}
	reader := bufio.NewReader(resp.Body)
	buffer := make([]byte, 32*1024)
	last := byte(0)
	wrote := false
	checked := false
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			if !checked {
				if !looksLikeJSON(buffer[:n]) {
					return errors.New("server returned a non-JSON success body")
				}
				checked = true
			}
			last = buffer[n-1]
			wrote = true
			if _, writeErr := out.Write(buffer[:n]); writeErr != nil {
				return &processError{err: writeErr}
			}
		}
		switch {
		case readErr == io.EOF:
			if wrote && last != '\n' {
				if _, writeErr := io.WriteString(out, "\n"); writeErr != nil {
					return &processError{err: writeErr}
				}
			}
			return nil
		case readErr != nil:
			if errors.Is(readErr, context.Canceled) {
				return readErr
			}
			if client.IsTimeout(readErr) {
				return &client.TimeoutError{Err: readErr}
			}
			return &processError{err: readErr}
		}
	}
}

// looksLikeJSON reports whether the first non-space byte can begin a JSON value.
func looksLikeJSON(chunk []byte) bool {
	for _, b := range chunk {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '{', '[', '"', '-', 't', 'f', 'n':
			return true
		default:
			return b >= '0' && b <= '9'
		}
	}
	return false
}

// writeJSONValue emits a locally built JSON value with HTML escaping disabled so
// URLs keep their exact characters.
func writeJSONValue(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return &processError{err: err}
	}
	return nil
}
