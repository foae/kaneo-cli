package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlainHTTPWarningPersistsAcrossInvocations(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session":null}`))
	})
	first := httptest.NewServer(handler)
	defer first.Close()
	second := httptest.NewServer(handler)
	defer second.Close()
	initial := newTestEnv(t)
	for index, server := range []*httptest.Server{first, first, second, second} {
		env := newTestEnv(t)
		env.dir = initial.dir
		env.env["KANEO_TOKEN"] = "synthetic-secret"
		env.env["KANEO_API_URL"] = server.URL + "/api"
		status, _, stderr := env.run("auth", "get-session")
		if status != 0 {
			t.Fatalf("invocation %d: status=%d stderr=%q", index, status, stderr)
		}
		warned := strings.Contains(stderr, "plain HTTP")
		if warned != (index%2 == 0) {
			t.Fatalf("invocation %d warning=%q", index, stderr)
		}
	}
}

func TestProfileResetRestoresPlainHTTPWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"session":null}`))
	}))
	defer server.Close()
	for _, reset := range []string{"logout", "delete"} {
		t.Run(reset, func(t *testing.T) {
			env := newTestEnv(t)
			env.env["KANEO_API_URL"] = server.URL + "/api"
			env.env["KANEO_TOKEN"] = "synthetic"
			if status, _, stderr := env.run("profile", "set", "default", "--api-url", server.URL+"/api"); status != 0 {
				t.Fatalf("set status=%d stderr=%s", status, stderr)
			}
			for attempt := range 2 {
				status, _, stderr := env.run("auth", "get-session")
				if status != 0 || strings.Contains(stderr, "plain HTTP") != (attempt == 0) {
					t.Fatalf("attempt=%d status=%d stderr=%s", attempt, status, stderr)
				}
			}
			args := []string{"auth", "logout", "--yes"}
			if reset == "delete" {
				args = []string{"profile", "delete", "default", "--yes"}
			}
			if status, _, stderr := env.run(args...); status != 0 {
				t.Fatalf("reset status=%d stderr=%s", status, stderr)
			}
			status, _, stderr := env.run("auth", "get-session")
			if status != 0 || !strings.Contains(stderr, "plain HTTP") {
				t.Fatalf("reset suppressed warning: status=%d stderr=%s", status, stderr)
			}
		})
	}
}
