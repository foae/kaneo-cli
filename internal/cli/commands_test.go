package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/foae/kaneo-cli/internal/auth"
	"github.com/foae/kaneo-cli/internal/buildinfo"
	"github.com/foae/kaneo-cli/internal/config"
)

type memoryKeyring struct {
	mu     sync.Mutex
	values map[string]string
	err    error
}

func newMemoryKeyring() *memoryKeyring { return &memoryKeyring{values: map[string]string{}} }

func (m *memoryKeyring) Name() string     { return config.BackendKeyring }
func (m *memoryKeyring) Location() string { return "memory keyring" }
func (m *memoryKeyring) key(p, u string) string {
	return p + "\x1f" + u
}

func (m *memoryKeyring) Set(profile, apiURL, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.values[m.key(profile, apiURL)] = secret
	return nil
}

func (m *memoryKeyring) Get(profile, apiURL string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	value, ok := m.values[m.key(profile, apiURL)]
	if !ok {
		return "", auth.ErrNotStored
	}
	return value, nil
}

func (m *memoryKeyring) Delete(profile, apiURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	key := m.key(profile, apiURL)
	if _, ok := m.values[key]; !ok {
		return auth.ErrNotStored
	}
	delete(m.values, key)
	return nil
}

type testEnv struct {
	app     *app
	dir     string
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
	keyring *memoryKeyring
	env     map[string]string
	stdin   *strings.Reader
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("")
	env := &testEnv{
		dir:     filepath.Join(t.TempDir(), "kaneo-cli"),
		stdout:  &stdout,
		stderr:  &stderr,
		keyring: newMemoryKeyring(),
		env:     map[string]string{},
		stdin:   stdin,
	}
	information := buildinfo.Info{Version: "v9.9.9", Commit: "abc", Date: "2026-09-20T00:00:00Z"}
	application := defaultApp(information, stdin, &stdout, &stderr)
	application.updateClient = nil
	application.configDir = func() (string, error) { return env.dir, nil }
	application.getenv = func(key string) string { return env.env[key] }
	application.credentialStore = func(cfg *config.Store, warn io.Writer) *auth.Store {
		return auth.NewStoreWithKeyring(cfg, env.keyring, warn)
	}
	env.app = application
	return env
}

func (e *testEnv) setStdin(value string) {
	e.stdin = strings.NewReader(value)
	e.app.streams.in = e.stdin
}

func (e *testEnv) run(args ...string) (int, string, string) {
	e.stdout.Reset()
	e.stderr.Reset()
	status := executeContext(context.Background(), e.app, args)
	return status, e.stdout.String(), e.stderr.String()
}

func TestInstanceGetStatusStreamsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/instance/status" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("public request carried Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hasUsers":false,"hasAdmin":false}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	status, stdout, stderr := env.run("instance", "get-status")
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, stderr)
	}
	if stdout != "{\"hasUsers\":false,\"hasAdmin\":false}\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if _, err := os.Stat(env.dir); !os.IsNotExist(err) {
		t.Fatalf("command created config dir %s", env.dir)
	}
}

func TestGetSessionNullAndNoContent(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "null session", status: http.StatusOK, body: "null", want: "null\n"},
		{name: "no content", status: http.StatusNoContent, body: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				if tt.body != "" {
					_, _ = w.Write([]byte(tt.body))
				}
			}))
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			status, stdout, stderr := env.run("auth", "get-session")
			if status != 0 {
				t.Fatalf("status = %d, stderr = %s", status, stderr)
			}
			if stdout != tt.want {
				t.Fatalf("stdout = %q, want %q", stdout, tt.want)
			}
		})
	}
}

func TestGetSessionRedactsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session":{"id":"s1","token":"SECRET","userId":"u1"},"user":{"id":"u1","email":"a@b.c"}}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("auth", "get-session")
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, stderr)
	}
	if strings.Contains(stdout, "SECRET") {
		t.Fatalf("stdout leaked the token: %q", stdout)
	}
	if !strings.Contains(stdout, `"[REDACTED]"`) {
		t.Fatalf("stdout = %q, want a redaction marker", stdout)
	}
	for _, want := range []string{"s1", "u1", "a@b.c"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want it to keep %q", stdout, want)
		}
	}
}

func TestGetSessionPassesThroughWithoutToken(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "null session value", body: `{"session":null}`},
		{name: "no session key", body: `{"user":{"id":"u1"}}`},
		{name: "session without token", body: `{"session":{"id":"s1"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			status, stdout, stderr := env.run("auth", "get-session")
			if status != 0 {
				t.Fatalf("status = %d, stderr = %s", status, stderr)
			}
			// The redacted path always re-encodes, so whitespace and
			// top-level key order may differ from the server's bytes.
			assertSameJSON(t, stdout, tt.body)
		})
	}
}

// assertSameJSON compares two JSON documents by value, ignoring whitespace and
// object key order, which the redacted path is free to change.
func assertSameJSON(t *testing.T, got, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("want not JSON: %v (%q)", err, want)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("stdout = %q, want equivalent to %q", got, want)
	}
}

// A secret-bearing path whose intermediate segment is present but is not an
// object cannot be redacted, so it must be rejected rather than printed.
func TestRedactedResponseRejectsNonObjectSegment(t *testing.T) {
	for _, body := range []string{`{"session":[{"token":"SECRET"}]}`, `{"session":"SECRET"}`} {
		t.Run(body, func(t *testing.T) {
			env := newRedactedSessionEnv(t, body)
			status, stdout, _ := env.run("auth", "get-session")
			if status == 0 {
				t.Fatalf("status = 0, want nonzero for a non-object session segment")
			}
			if strings.Contains(stdout, "SECRET") {
				t.Fatalf("stdout leaked the token: %q", stdout)
			}
		})
	}
}

// Go's decoder keeps the last duplicate key, so the output must be re-encoded
// from the decoded value; passing the original bytes through would emit the
// earlier, secret-bearing copy.
func TestRedactedResponseHandlesDuplicateKeys(t *testing.T) {
	t.Run("later duplicate is null", func(t *testing.T) {
		env := newRedactedSessionEnv(t, `{"session":{"token":"SECRET"},"session":null}`)
		status, stdout, stderr := env.run("auth", "get-session")
		if status != 0 {
			t.Fatalf("status = %d, stderr = %s", status, stderr)
		}
		if strings.Contains(stdout, "SECRET") {
			t.Fatalf("stdout leaked the token: %q", stdout)
		}
		assertSameJSON(t, stdout, `{"session":null}`)
	})
	t.Run("both duplicates bear a secret", func(t *testing.T) {
		env := newRedactedSessionEnv(t, `{"session":{"token":"A"},"session":{"token":"B"}}`)
		status, stdout, stderr := env.run("auth", "get-session")
		if status != 0 {
			t.Fatalf("status = %d, stderr = %s", status, stderr)
		}
		for _, secret := range []string{`"A"`, `"B"`} {
			if strings.Contains(stdout, secret) {
				t.Fatalf("stdout leaked %s: %q", secret, stdout)
			}
		}
		if !strings.Contains(stdout, `"[REDACTED]"`) {
			t.Fatalf("stdout = %q, want a redaction marker", stdout)
		}
	})
	t.Run("duplicate inside the session object", func(t *testing.T) {
		env := newRedactedSessionEnv(t, `{"session":{"token":"SECRET","token":null}}`)
		status, stdout, stderr := env.run("auth", "get-session")
		if status != 0 {
			t.Fatalf("status = %d, stderr = %s", status, stderr)
		}
		if strings.Contains(stdout, "SECRET") {
			t.Fatalf("stdout leaked the token: %q", stdout)
		}
	})
}

// The legacy single-field path is used for gitea get-integration, which must
// not leak an earlier duplicate of the redacted field either.
func TestGiteaIntegrationRedactsDuplicateWebhookSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webhookSecret":"SECRET","webhookSecret":null}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("gitea", "get-integration", "--project-id", "p1")
	if status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, stderr)
	}
	if strings.Contains(stdout, "SECRET") {
		t.Fatalf("stdout leaked the webhook secret: %q", stdout)
	}
	assertSameJSON(t, stdout, `{"webhookSecret":null}`)
}

// newRedactedSessionEnv serves body from a stub API for auth get-session.
func newRedactedSessionEnv(t *testing.T, body string) *testEnv {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	return env
}

// A secret-bearing response that is valid JSON but not an object could itself
// be the secret, so it must be rejected rather than printed. Only a bare null
// passes through, because an unauthenticated session is legitimately null.
func TestRedactedResponseRejectsNonObjectBody(t *testing.T) {
	for _, body := range []string{`"bare-secret-string"`, `["bare-secret-string"]`, `123`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			status, stdout, _ := env.run("auth", "get-session")
			if status == 0 {
				t.Fatalf("status = 0, want nonzero for a non-object secret-bearing body")
			}
			if strings.Contains(stdout, "bare-secret-string") {
				t.Fatalf("stdout leaked the body: %q", stdout)
			}
		})
	}
}

func TestUnauthorizedExitCodeAndErrorShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized","message":"Invalid key"}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("auth", "get-session")
	if status != 3 {
		t.Fatalf("status = %d, want 3", status)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	var response errorResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatalf("stderr not JSON: %v (%q)", err, stderr)
	}
	if response.Error.Code != "authentication_failed" || response.Error.Status != 401 {
		t.Fatalf("error = %+v", response.Error)
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	env := newTestEnv(t)
	status, stdout, stderr := env.run("instance", "get-status", "extra")
	if status != 2 || stdout != "" {
		t.Fatalf("status = %d, stdout = %q", status, stdout)
	}
	if !strings.Contains(stderr, "invalid_arguments") {
		t.Fatalf("stderr = %q", stderr)
	}

	status, _, stderr = env.run("--timeout", "bogus", "instance", "get-status")
	if status != 2 || !strings.Contains(stderr, "invalid_arguments") {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}

	status, _, stderr = env.run("profile", "set", "a", "b", "c")
	if status != 2 || !strings.Contains(stderr, "invalid_arguments") {
		t.Fatalf("argument-count status = %d, stderr = %q", status, stderr)
	}
}

func TestKANEO_TOKENIsInvocationOnly(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"hasUsers":true,"hasAdmin":true}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "invocation-token"
	if status, _, stderr := env.run("auth", "get-session"); status != 0 {
		t.Fatalf("status = %d, stderr = %s", status, stderr)
	}
	if gotAuth != "Bearer invocation-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if _, err := os.Stat(filepath.Join(env.dir, "credentials.json")); !os.IsNotExist(err) {
		t.Fatal("KANEO_TOKEN was persisted")
	}
}

func TestProfileIsolationAndCredentialBinding(t *testing.T) {
	var mu sync.Mutex
	var lastAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"hasUsers":true,"hasAdmin":true}`))
	}))
	defer server.Close()
	base := server.URL + "/api"

	env := newTestEnv(t)
	if status, _, stderr := env.run("profile", "set", "a", "--api-url", base); status != 0 {
		t.Fatalf("profile set a status = %d: %s", status, stderr)
	}
	if status, _, stderr := env.run("profile", "set", "b", "--api-url", base); status != 0 {
		t.Fatalf("profile set b status = %d: %s", status, stderr)
	}

	env.setStdin("key-a\n")
	if status, _, stderr := env.run("--profile", "a", "auth", "login", "--api-key-file", "-"); status != 0 {
		t.Fatalf("login a status = %d: %s", status, stderr)
	}
	env.setStdin("key-b\n")
	if status, _, stderr := env.run("--profile", "b", "auth", "login", "--api-key-file", "-"); status != 0 {
		t.Fatalf("login b status = %d: %s", status, stderr)
	}

	if status, _, stderr := env.run("--profile", "a", "auth", "get-session"); status != 0 {
		t.Fatalf("profile a request failed: %s", stderr)
	}
	mu.Lock()
	authA := lastAuth
	mu.Unlock()
	if authA != "Bearer key-a" {
		t.Fatalf("profile a Authorization = %q", authA)
	}

	if status, _, stderr := env.run("--profile", "b", "auth", "get-session"); status != 0 {
		t.Fatalf("profile b request failed: %s", stderr)
	}
	mu.Lock()
	authB := lastAuth
	mu.Unlock()
	if authB != "Bearer key-b" {
		t.Fatalf("profile b Authorization = %q", authB)
	}
}

func TestCredentialNotTransplantedAcrossURLs(t *testing.T) {
	var mu sync.Mutex
	var lastAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"hasUsers":true,"hasAdmin":true}`))
	}))
	defer server.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"hasUsers":true,"hasAdmin":true}`))
	}))
	defer other.Close()

	env := newTestEnv(t)
	if status, _, stderr := env.run("profile", "set", "a", "--api-url", server.URL+"/api"); status != 0 {
		t.Fatalf("profile set status = %d: %s", status, stderr)
	}
	env.setStdin("key-a\n")
	if status, _, stderr := env.run("--profile", "a", "auth", "login", "--api-key-file", "-"); status != 0 {
		t.Fatalf("login status = %d: %s", status, stderr)
	}

	// A different API URL must not receive the credential bound to the original.
	if status, _, stderr := env.run("--profile", "a", "--api-url", other.URL+"/api", "auth", "get-session"); status != 0 {
		t.Fatalf("override status = %d: %s", status, stderr)
	}
	mu.Lock()
	defer mu.Unlock()
	if lastAuth != "" {
		t.Fatalf("credential was transplanted to another URL: %q", lastAuth)
	}
}

func TestProfileDeleteRequiresYes(t *testing.T) {
	env := newTestEnv(t)
	if status, _, _ := env.run("profile", "set", "a", "--api-url", "https://example.com/api"); status != 0 {
		t.Fatal("profile set failed")
	}
	status, _, stderr := env.run("profile", "delete", "a")
	if status != 2 {
		t.Fatalf("status = %d, want 2 for missing --yes", status)
	}
	if !strings.Contains(stderr, "invalid_arguments") {
		t.Fatalf("stderr = %q", stderr)
	}
	if status, _, stderr := env.run("profile", "delete", "a", "--yes"); status != 0 {
		t.Fatalf("delete with --yes status = %d: %s", status, stderr)
	}
}

func TestHelpAndVersionDoNotCreateConfig(t *testing.T) {
	env := newTestEnv(t)
	if status, _, _ := env.run("--help"); status != 0 {
		t.Fatal("help failed")
	}
	if status, stdout, _ := env.run("version"); status != 0 || !strings.Contains(stdout, "v9.9.9") {
		t.Fatalf("version status = %d, stdout = %q", status, stdout)
	}
	if _, err := os.Stat(env.dir); !os.IsNotExist(err) {
		t.Fatalf("config dir created: %s", env.dir)
	}
}

func TestCrossOriginRedirectRefused(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/elsewhere", http.StatusFound)
	}))
	defer source.Close()

	env := newTestEnv(t)
	env.setAPIURL(source.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("auth", "get-session")
	if status != 4 {
		t.Fatalf("status = %d, want 4 (stderr = %s)", status, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "cross_origin_redirect_refused") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestTimeoutMapsToExitFour(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	status, _, stderr := env.run("--timeout", "40ms", "instance", "get-status")
	if status != 4 {
		t.Fatalf("status = %d, want 4 (stderr = %s)", status, stderr)
	}
	if !strings.Contains(stderr, "\"timeout\"") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCancellationMapsToExit130(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	env.app.streams.out = stdout
	env.app.streams.errOut = stderr
	status := executeContext(ctx, env.app, []string{"instance", "get-status"})
	if status != 130 {
		t.Fatalf("status = %d, want 130 (stderr = %s)", status, stderr.String())
	}
	if !strings.Contains(stderr.String(), "interrupted") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestLoginDeviceStoresCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer device-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"hasUsers":true,"hasAdmin":true}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.app.deviceLogin = func(_ context.Context, opts auth.DeviceOptions) (auth.DeviceToken, error) {
		if opts.ClientID != auth.DefaultClientID {
			t.Errorf("client id = %q, want %q", opts.ClientID, auth.DefaultClientID)
		}
		return auth.DeviceToken{AccessToken: "device-token", TokenType: "Bearer"}, nil
	}
	status, stdout, stderr := env.run("auth", "login")
	if status != 0 {
		t.Fatalf("login status = %d: %s", status, stderr)
	}
	if !strings.Contains(stdout, "\"storage\":\"keyring\"") {
		t.Fatalf("stdout = %q", stdout)
	}
	if status, _, stderr := env.run("auth", "get-session"); status != 0 {
		t.Fatalf("request after login status = %d: %s", status, stderr)
	}
}

func TestAPIKeyFilePermissionPolicy(t *testing.T) {
	env := newTestEnv(t)
	env.setAPIURL("https://example.com/api")

	if runtime.GOOS != "windows" {
		lax := filepath.Join(t.TempDir(), "key.txt")
		if err := os.WriteFile(lax, []byte("secret-value\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if status, _, stderr := env.run("auth", "login", "--api-key-file", lax); status != 2 {
			t.Fatalf("lax file status = %d, want 2 (stderr = %s)", status, stderr)
		}
	}

	strict := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(strict, []byte("secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, _, stderr := env.run("auth", "login", "--api-key-file", strict); status != 0 {
		t.Fatalf("strict file status = %d: %s", status, stderr)
	}
}

func TestLogoutRemovesCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	base := server.URL + "/api"
	if status, _, _ := env.run("profile", "set", "a", "--api-url", base); status != 0 {
		t.Fatal("profile set failed")
	}
	env.setStdin("key-a\n")
	if status, _, stderr := env.run("--profile", "a", "auth", "login", "--api-key-file", "-"); status != 0 {
		t.Fatalf("login status = %d: %s", status, stderr)
	}
	if status, _, stderr := env.run("--profile", "a", "auth", "logout", "--yes"); status != 0 {
		t.Fatalf("logout status = %d: %s", status, stderr)
	}
	if len(env.keyring.values) != 0 {
		t.Fatalf("keyring still holds %v", env.keyring.values)
	}
}

// setAPIURL supplies the base URL for commands that do not set a profile.
func (e *testEnv) setAPIURL(rawURL string) {
	e.env["KANEO_API_URL"] = rawURL
}

func TestTransportFailureMapsToExitFour(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rawURL := server.URL
	server.Close()

	env := newTestEnv(t)
	env.setAPIURL(rawURL + "/api")
	status, stdout, stderr := env.run("instance", "get-status")
	if status != 4 {
		t.Fatalf("status = %d, want 4 (stderr = %s)", status, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "transport_failure") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestDeviceDenialMapsToExitThree(t *testing.T) {
	env := newTestEnv(t)
	env.setAPIURL("https://example.com/api")
	env.app.deviceLogin = func(context.Context, auth.DeviceOptions) (auth.DeviceToken, error) {
		return auth.DeviceToken{}, auth.ErrDeviceAccessDenied
	}
	status, stdout, stderr := env.run("auth", "login")
	if status != 3 {
		t.Fatalf("status = %d, want 3 (stderr = %s)", status, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "access_denied") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestNonJSONSuccessRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	status, stdout, stderr := env.run("auth", "get-session")
	if status != 1 {
		t.Fatalf("status = %d, want 1 (stderr = %s)", status, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
}

func TestLogoutRequiresYes(t *testing.T) {
	env := newTestEnv(t)
	env.setAPIURL("https://example.com/api")
	env.setStdin("key-a\n")
	if status, _, stderr := env.run("auth", "login", "--api-key-file", "-"); status != 0 {
		t.Fatalf("login status = %d: %s", status, stderr)
	}
	if status, _, stderr := env.run("auth", "logout"); status != 2 {
		t.Fatalf("logout status = %d, want 2 (stderr = %s)", status, stderr)
	}
	if len(env.keyring.values) == 0 {
		t.Fatal("credential removed without --yes")
	}
}

func TestNamedProfileLoginPersistsOnlyAfterSuccess(t *testing.T) {
	env := newTestEnv(t)
	const base = "https://example.com/api"
	env.setAPIURL(base)
	if status, _, stderr := env.run("profile", "set", "stable", "--api-url", base); status != 0 {
		t.Fatalf("set stable profile: status=%d stderr=%q", status, stderr)
	}

	store := config.NewStore(env.dir)
	before, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("read initial config: %v", err)
	}
	assertConfigUnchanged := func(t *testing.T) {
		t.Helper()
		after, err := os.ReadFile(store.Path())
		if err != nil {
			t.Fatalf("read config after failed login: %v", err)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("failed login changed config:\nbefore=%s\nafter=%s", before, after)
		}
	}

	env.setStdin("\n")
	if status, _, stderr := env.run("--profile", "team", "auth", "login", "--api-key-file", "-"); status != 2 {
		t.Fatalf("empty API key status=%d, want 2 (stderr=%q)", status, stderr)
	}
	assertConfigUnchanged(t)

	env.app.deviceLogin = func(context.Context, auth.DeviceOptions) (auth.DeviceToken, error) {
		return auth.DeviceToken{}, auth.ErrDeviceAccessDenied
	}
	if status, _, stderr := env.run("--profile", "team", "auth", "login"); status != 3 {
		t.Fatalf("denied device login status=%d, want 3 (stderr=%q)", status, stderr)
	}
	assertConfigUnchanged(t)

	env.setStdin("team-key\n")
	if status, _, stderr := env.run("--profile", "team", "auth", "login", "--api-key-file", "-"); status != 0 {
		t.Fatalf("successful named login: status=%d stderr=%q", status, stderr)
	}
	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load config after successful login: %v", err)
	}
	if cfg.DefaultProfile != "stable" {
		t.Fatalf("default profile=%q, want stable", cfg.DefaultProfile)
	}
	profile, ok := cfg.Profile("team")
	if !ok {
		t.Fatal("successful named login did not create profile")
	}
	if profile.APIURL != base {
		t.Fatalf("team API URL=%q, want %q", profile.APIURL, base)
	}
	ref, ok := profile.CredentialFor(base)
	if !ok || ref.Backend != config.BackendKeyring {
		t.Fatalf("team credential reference=%+v, stored=%t", ref, ok)
	}
	if secret, err := env.keyring.Get("team", base); err != nil || secret != "team-key" {
		t.Fatalf("team credential=%q, err=%v", secret, err)
	}
}

func TestLoginCreatesExplicitProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hasUsers":true,"hasAdmin":true}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	base := server.URL + "/api"
	env.env["KANEO_API_URL"] = base
	env.setStdin("key-a\n")
	if status, _, stderr := env.run("auth", "login", "--api-key-file", "-", "--profile", "team"); status != 0 {
		t.Fatalf("login: status=%d stderr=%q", status, stderr)
	}
	if status, stdout, stderr := env.run("instance", "get-status", "--profile", "team"); status != 0 {
		t.Fatalf("reuse profile: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

func TestProfileCommandsWorkWhenBackendFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"hasUsers":true,"hasAdmin":true}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	base := server.URL + "/api"
	env.env["KANEO_API_URL"] = base
	env.setStdin("key-a\n")
	if status, _, stderr := env.run("auth", "login", "--api-key-file", "-"); status != 0 {
		t.Fatalf("login status = %d: %s", status, stderr)
	}
	// The stored credential becomes inaccessible after login.
	env.keyring.mu.Lock()
	env.keyring.err = errors.New("AccessDenied: keyring locked")
	env.keyring.mu.Unlock()

	if status, _, stderr := env.run("profile", "list"); status != 0 {
		t.Fatalf("profile list status = %d: %s", status, stderr)
	}
	if status, _, _ := env.run("instance", "get-status"); status != 0 {
		t.Fatal("public read failed because the credential backend is unavailable")
	}
}
