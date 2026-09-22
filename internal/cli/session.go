package cli

import (
	"bytes"
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
	ctx      context.Context
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
		ctx:      ctx,
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
		WarnWriter: s.store.WarningWriter(s.ctx, "plain-http:"+s.resolved.ProfileName+":"+s.resolved.Base.Origin(), s.app.streams.errOut),
		UserAgent:  "kaneo-cli/" + s.app.info.Version,
	})
	if err != nil {
		return nil, &processError{err: err}
	}
	s.warnDefaultAPI()
	return created, nil
}

func (s *session) warnDefaultAPI() {
	if s.resolved.Sources["api_url"] != "default" || s.app.warnedDefaultAPI {
		return
	}
	s.app.warnedDefaultAPI = true
	_, _ = fmt.Fprintf(s.app.streams.errOut, "warning: using the implicit Kaneo Cloud API default (%s); for self-hosted instances set --api-url or KANEO_API_URL to the full API base URL (normally ending in /api). Inspect effective settings with kaneo-cli profile get.\n", config.DefaultAPIURL)
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
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRequestBody+1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if client.IsTimeout(err) {
			return &client.TimeoutError{Err: err}
		}
		return &processError{err: err}
	}
	if len(payload) > maxRequestBody {
		return &processError{err: errors.New("response exceeds size limit")}
	}
	if !json.Valid(payload) {
		return client.ErrInvalidJSONResponse
	}
	return writeJSONBytes(out, payload)
}

// writeRedactedJSON redacts a single top-level field. It buffers before writing
// so malformed responses cannot leak secrets, and always re-encodes the body
// rather than passing the server's bytes through, so a duplicate key cannot
// smuggle a secret-bearing copy past the redaction. RawMessage preserves
// unrelated values, including precise numbers; whitespace and key order may
// change.
func writeRedactedJSON(out io.Writer, resp *client.Response, field string) error {
	return writeRedactedJSONPath(out, resp, field)
}

// writeRedactedJSONPath redacts the value at a nested path, descending one key
// at a time. It buffers before writing so malformed responses cannot leak
// secrets, and the body is always re-encoded from the decoded object, never
// passed through as received: whitespace and key order may change, and a
// duplicate key cannot leave an unredacted copy in the output. RawMessage
// preserves unrelated values, including precise numbers.
// A bare null body passes through as null, because an unauthenticated session
// is legitimately null. An absent or null segment leaves the body unredacted
// but still re-encoded. Any other non-object shape, at the top level or at an
// intermediate segment, is rejected rather than printed, because it could
// itself be the secret.
func writeRedactedJSONPath(out io.Writer, resp *client.Response, path ...string) error {
	defer func() { _ = resp.Body.Close() }()
	if resp.Empty() {
		return nil
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRequestBody+1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if client.IsTimeout(err) {
			return &client.TimeoutError{Err: err}
		}
		return &processError{err: errors.New("could not read secret-bearing response")}
	}
	if len(payload) > maxRequestBody {
		return &processError{err: errors.New("secret-bearing response exceeds size limit")}
	}
	if len(path) == 0 {
		// No caller asks for this; fail closed rather than print a
		// secret-bearing body with nothing redacted.
		return client.ErrInvalidJSONResponse
	}
	// A bare null unmarshals into a nil map without error, so an
	// unauthenticated session reaches redactPath and re-encodes to null. Any
	// other non-object body could itself be the secret, so it stays rejected.
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		return client.ErrInvalidJSONResponse
	}
	if redactPath(object, path) == redactReject {
		return client.ErrInvalidJSONResponse
	}
	return writeJSONValue(out, object)
}

// redactOutcome distinguishes a redaction that was applied, a body that safely
// needed none, and a body whose shape means it cannot be trusted to print.
type redactOutcome int

const (
	redactUnchanged redactOutcome = iota
	redactApplied
	redactReject
)

// redactPath walks object one segment at a time and replaces the value at the
// final segment. A missing or null segment is unchanged and safe; an
// intermediate segment that is present but is not a JSON object is rejected,
// because the caller cannot redact inside it and must not print it.
func redactPath(object map[string]json.RawMessage, path []string) redactOutcome {
	field := path[len(path)-1]
	if len(path) == 1 {
		value, ok := object[field]
		if !ok || string(value) == "null" {
			return redactUnchanged
		}
		object[field] = json.RawMessage(`"[REDACTED]"`)
		return redactApplied
	}
	head := path[0]
	raw, ok := object[head]
	if !ok || string(raw) == "null" {
		return redactUnchanged
	}
	var child map[string]json.RawMessage
	if err := json.Unmarshal(raw, &child); err != nil {
		return redactReject
	}
	outcome := redactPath(child, path[1:])
	if outcome == redactReject {
		return redactReject
	}
	// The child is re-encoded even when nothing changed, so a duplicate key
	// inside its original bytes cannot carry an unredacted copy into the
	// output.
	encoded, err := marshalNoEscape(child)
	if err != nil {
		return redactReject
	}
	object[head] = encoded
	return outcome
}

// marshalNoEscape encodes a nested value with HTML escaping disabled so URLs
// keep their exact characters, matching writeJSONValue.
func marshalNoEscape(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// writeJSONBytes copies already-validated JSON bytes to stdout with a trailing
// newline, preserving the server's exact encoding.
func writeJSONBytes(out io.Writer, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	if _, err := out.Write(payload); err != nil {
		return &processError{err: err}
	}
	if payload[len(payload)-1] != '\n' {
		if _, err := io.WriteString(out, "\n"); err != nil {
			return &processError{err: err}
		}
	}
	return nil
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
