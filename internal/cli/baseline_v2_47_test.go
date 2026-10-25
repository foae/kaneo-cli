package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// Kaneo 2.47.0 dropped the 1024-character cap on MCP OAuth state; only the
// server's 8 KiB raw-URL bound remains, answered with HTTP 414.
func TestV247MCPAuthorizationPreservesLongOpaqueState(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")

	state := strings.Repeat("s", 2048) + "+/%=& ü"
	status, stdout, stderr := env.run(append(mcpArgs(), "--state", state)...)
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	printed, err := url.Parse(strings.TrimSuffix(stdout, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := printed.Query().Get("state"); got != state {
		t.Fatalf("state changed: got %d bytes want %d", len(got), len(state))
	}
	if requests.Load() != 0 {
		t.Fatalf("requests=%d, want 0", requests.Load())
	}
}

func TestV247WorkflowRuleHelpListsEventTypes(t *testing.T) {
	env := newTestEnv(t)
	status, stdout, stderr := env.run("workflow-rule", "upsert-project", "--help")
	want := `eventType (required): string (one of: "branch_push", "pr_opened", "pr_merged", "issue_opened", "issue_closed", "issue_reopened")`
	if status != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

// Request-body validation stays syntactic: the server, not the CLI, rejects an
// event type outside the 2.47.0 enum.
func TestV247WorkflowRuleBodyForwardedUnchanged(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   string
		code   int
		status int
	}{
		{"enum value", `{"integrationType":"github","eventType":"pr_opened","columnId":"c1"}`, 200, 0},
		{"server rejects legacy provider event", `{"integrationType":"github","eventType":"pull_request.opened","columnId":"c1"}`, 400, 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				body, err := io.ReadAll(r.Body)
				if r.Method != http.MethodPut || r.URL.Path != "/api/workflow-rule/p1" || err != nil || string(body) != test.body {
					t.Errorf("request=%s %s body=%q err=%v", r.Method, r.URL.Path, body, err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.code)
				if test.code == 200 {
					_, _ = w.Write([]byte(`{"id":"r1","projectId":"p1","integrationType":"github","eventType":"pr_opened","columnId":"c1"}`))
					return
				}
				_, _ = w.Write([]byte(`{"code":"VALIDATION_ERROR","message":"Invalid request"}`))
			}))
			defer server.Close()
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			env.setStdin(test.body)
			status, stdout, stderr := env.run("workflow-rule", "upsert-project", "--project-id", "p1", "--body-file", "-")
			if status != test.status || requests.Load() != 1 {
				t.Fatalf("status=%d want=%d requests=%d stdout=%q stderr=%q", status, test.status, requests.Load(), stdout, stderr)
			}
			if (test.status == 0) == (stdout == "") {
				t.Fatalf("stdout=%q for status %d", stdout, status)
			}
		})
	}
}
