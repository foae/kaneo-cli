package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// This list is deliberately independent of writeSpecs: changing a safety bit
// must not silently change which commands the confirmation regression covers.
func v249ConfirmedMutationArgs() [][]string {
	return [][]string{
		{"admin", "archive-workspace", "--workspace-id", "w1"},
		{"admin", "create-user"},
		{"admin", "create-role"},
		{"admin", "create-webhook"},
		{"admin", "deactivate-user", "--user-id", "u1"},
		{"admin", "delete-user", "--user-id", "u1"},
		{"admin", "delete-workspace", "--workspace-id", "w1"},
		{"admin", "delete-role", "--role-id", "r1"},
		{"admin", "delete-webhook", "--webhook-id", "h1"},
		{"admin", "mark-user-email-verified", "--user-id", "u1"},
		{"admin", "reactivate-user", "--user-id", "u1"},
		{"admin", "remove-user-avatar", "--user-id", "u1"},
		{"admin", "restore-workspace", "--workspace-id", "w1"},
		{"admin", "revoke-user-api-key", "--user-id", "u1", "--api-key-id", "k1"},
		{"admin", "revoke-user-session", "--user-id", "u1", "--session-id", "s1"},
		{"admin", "revoke-user-sessions", "--user-id", "u1"},
		{"user", "revoke-session", "--session-id", "s1"},
		{"admin", "rotate-webhook-secret", "--webhook-id", "h1"},
		{"admin", "send-user-password-reset", "--user-id", "u1"},
		{"admin", "set-user-avatar", "--user-id", "u1"},
		{"admin", "update-user", "--user-id", "u1"},
		{"admin", "update-role", "--role-id", "r1"},
		{"admin", "update-settings"},
		{"admin", "update-webhook", "--webhook-id", "h1"},
		{"admin", "add-workspace-member", "--workspace-id", "w1"},
		{"admin", "update-workspace-member-role", "--workspace-id", "w1", "--user-id", "u1"},
	}
}

func TestV249ConfirmationPrecedesCredentialsFilesAndNetwork(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	for _, args := range v249ConfirmedMutationArgs() {
		t.Run(strings.Join(args[:2], "/"), func(t *testing.T) {
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			// No credential and no body/file: confirmation must win over both.
			status, stdout, stderr := env.run(args...)
			if status != 2 || stdout != "" || !strings.Contains(stderr, "without --yes") || requests.Load() != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%d", status, stdout, stderr, requests.Load())
			}
		})
	}
}

func TestV249MutationParameterConstraintsBeforeNetwork(t *testing.T) {
	cases := [][]string{
		{"admin", "revoke-user-api-key", "--user-id", strings.Repeat("u", 201), "--api-key-id", "k1", "--yes"},
		{"admin", "revoke-user-api-key", "--user-id", "u1", "--api-key-id", strings.Repeat("界", 201), "--yes"},
		{"admin", "revoke-user-session", "--user-id", "u1", "--session-id", strings.Repeat("s", 201), "--yes"},
		{"admin", "revoke-user-sessions", "--user-id", strings.Repeat("u", 201), "--yes"},
		{"user", "revoke-session", "--session-id", strings.Repeat("s", 201), "--yes"},
		{"admin", "update-role", "--role-id=", "--yes", "--body-file", "-"},
		{"admin", "deactivate-user", "--user-id", "..", "--yes"},
		{"admin", "archive-workspace", "--workspace-id", ".", "--yes"},
		{"admin", "rotate-webhook-secret", "--webhook-id=", "--yes"},
		{"external-link", "update-task", "--task-id", "t1", "--id", "..", "--body-file", "-"},
		{"task", "update-recurrence", "--id=", "--body-file", "-"},
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	for i, args := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			env.setStdin(`{}`)
			status, stdout, stderr := env.run(args...)
			if status != 2 || stdout != "" || !strings.Contains(stderr, "invalid_arguments") || requests.Load() != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%d", status, stdout, stderr, requests.Load())
			}
		})
	}
}

func TestV249WebhookSecretsRedactedBeforeOutput(t *testing.T) {
	for _, args := range [][]string{
		{"admin", "create-webhook", "--yes", "--body-file", "-"},
		{"admin", "rotate-webhook-secret", "--webhook-id", "h1", "--yes"},
	} {
		for _, response := range []struct {
			name, body string
			valid      bool
		}{
			{"object", `{"webhook":{"id":"h1"},"secret":"synthetic-secret-first","secret":"synthetic-secret-second","counter":12345678901234567890}`, true},
			{"string", `"synthetic-secret-first"`, false},
			{"array", `["synthetic-secret-first"]`, false},
			{"malformed", `{"secret":"synthetic-secret-first"`, false},
		} {
			t.Run(args[1]+"/"+response.name, func(t *testing.T) {
				var requests atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, response.body)
				}))
				defer server.Close()
				env := newTestEnv(t)
				env.setAPIURL(server.URL + "/api")
				env.env["KANEO_TOKEN"] = "synthetic"
				env.setStdin(`{"url":"https://hooks.example.test","categories":["user"],"enabled":true}`)
				status, stdout, stderr := env.run(args...)
				if requests.Load() != 1 || strings.Contains(stdout+stderr, "synthetic-secret-") {
					t.Fatalf("secret escaped or request retried: requests=%d stdout=%q stderr=%q", requests.Load(), stdout, stderr)
				}
				if !response.valid {
					if status == 0 || stdout != "" {
						t.Fatalf("unsafe response printed: status=%d stdout=%q stderr=%q", status, stdout, stderr)
					}
					return
				}
				var object map[string]json.RawMessage
				if err := json.Unmarshal([]byte(stdout), &object); err != nil {
					t.Fatal(err)
				}
				if status != 0 || string(object["secret"]) != `"[REDACTED]"` || string(object["counter"]) != "12345678901234567890" || string(object["webhook"]) != `{"id":"h1"}` {
					t.Fatalf("redaction damaged output: status=%d stdout=%q stderr=%q", status, stdout, stderr)
				}
			})
		}
	}
}

func TestV249MutationFailuresRemainSingleRequest(t *testing.T) {
	argsList := v249ConfirmedMutationArgs()
	argsList = append(argsList,
		[]string{"admin", "send-test-email"},
		[]string{"admin", "send-user-verification-email", "--user-id", "u1"},
		[]string{"admin", "test-webhook", "--webhook-id", "h1"},
		[]string{"external-link", "update-task", "--task-id", "t1", "--id", "l1"},
		[]string{"task", "update-recurrence", "--id", "t1"},
	)
	image := filepath.Join(t.TempDir(), "avatar.png")
	if err := os.WriteFile(image, []byte("synthetic-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range argsList {
		for _, code := range []int{401, 403, 409, 429} {
			t.Run(strings.Join(args[:2], "/")+"/"+strconv.Itoa(code), func(t *testing.T) {
				var requests atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Header.Get("Authorization") != "Bearer synthetic" {
						t.Error("configured credential missing")
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(code)
					_, _ = io.WriteString(w, `{"code":"access_rejected","message":"Request rejected","secret":"synthetic-response-secret"}`)
				}))
				defer server.Close()
				env := newTestEnv(t)
				env.setAPIURL(server.URL + "/api")
				env.env["KANEO_TOKEN"] = "synthetic"
				env.setStdin(`{}`)
				runArgs := append(append([]string{}, args...), "--yes")
				root := env.app.newRootCommand()
				cmd := findCommand(findCommand(root, args[0]), args[1])
				if cmd.Flags().Lookup("body-file") != nil {
					runArgs = append(runArgs, "--body-file", "-")
				}
				if args[1] == "set-user-avatar" {
					runArgs = append(runArgs, "--file", image)
				}
				status, stdout, stderr := env.run(runArgs...)
				want := 5
				if code == 401 || code == 403 {
					want = 3
				}
				wantCode := map[int]string{401: "authentication_failed", 403: "authorization_failed", 409: "conflict", 429: "rate_limited"}[code]
				if status != want || stdout != "" || requests.Load() != 1 || !strings.Contains(stderr, `"code":"`+wantCode+`"`) || strings.Contains(stderr, "synthetic-response-secret") {
					t.Fatalf("status=%d want=%d stdout=%q stderr=%q requests=%d", status, want, stdout, stderr, requests.Load())
				}
			})
		}
	}
}

func TestV249AdminAvatarUsesBoundedFileBytes(t *testing.T) {
	for _, contentType := range []string{"image/png", "image/jpeg", "image/webp"} {
		t.Run(contentType, func(t *testing.T) {
			data := bytes.Repeat([]byte{0, 0xff, 0x80, 'x'}, maxAvatarBytes/4)
			path := filepath.Join(t.TempDir(), "avatar.bin")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != "PUT" || r.URL.EscapedPath() != "/api/admin/users/u%2F1/avatar" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer synthetic" {
					t.Errorf("incorrect upload request: %s %s", r.Method, r.URL)
				}
				var body struct {
					ContentType string `json:"contentType"`
					Data        string `json:"data"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				decoded, err := base64.StdEncoding.DecodeString(body.Data)
				if err != nil || !bytes.Equal(decoded, data) || body.ContentType != contentType {
					t.Error("upload changed file bytes or content type")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"image":"/api/user/avatar/u1"}`)
			}))
			defer server.Close()
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			status, stdout, stderr := env.run("admin", "set-user-avatar", "--user-id", "u/1", "--file", path, "--content-type", contentType, "--yes")
			if status != 0 || requests.Load() != 1 || stdout != "{\"image\":\"/api/user/avatar/u1\"}\n" {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%d", status, stdout, stderr, requests.Load())
			}
			if strings.Contains(stdout+stderr, base64.StdEncoding.EncodeToString(data)) || strings.Contains(stdout+stderr, "uploadUrl") {
				t.Fatal("upload bytes or transfer credentials escaped")
			}
		})
	}
}

func TestV249AdminAvatarValidationBeforeNetwork(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.png")
	oversized := filepath.Join(dir, "large.png")
	valid := filepath.Join(dir, "avatar.png")
	for path, data := range map[string][]byte{empty: {}, oversized: bytes.Repeat([]byte{1}, maxAvatarBytes+1), valid: {1}} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	for i, args := range [][]string{
		{"--file", valid},
		{"--user-id=", "--file", valid},
		{"--user-id", "..", "--file", valid},
		{"--user-id", "u1"},
		{"--user-id", "u1", "--file", empty},
		{"--user-id", "u1", "--file", oversized},
		{"--user-id", "u1", "--file", dir, "--content-type", "image/png"},
		{"--user-id", "u1", "--file", valid, "--content-type", "image/gif"},
		{"--user-id", "u1", "--body-file", "-"},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			status, stdout, stderr := env.run(append([]string{"admin", "set-user-avatar", "--yes"}, args...)...)
			if status != 2 || stdout != "" || !strings.Contains(stderr, "invalid_arguments") || requests.Load() != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%d", status, stdout, stderr, requests.Load())
			}
		})
	}
}
