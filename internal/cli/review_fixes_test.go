package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runInterrupted runs args and cancels the invocation's context once started
// is closed, returning the exit status and captured streams.
func (e *testEnv) runInterrupted(started <-chan struct{}, args ...string) (int, string, string) {
	e.stdout.Reset()
	e.stderr.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()
	status := executeContext(ctx, e.app, args)
	return status, e.stdout.String(), e.stderr.String()
}

func TestInterruptedCalendarFeedDownloadHidesToken(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	status, stdout, stderr := env.runInterrupted(started,
		"calendar-feed", "download", "--token-file", writeTokenFile(t, feedToken), "--output", "-")
	if status != 130 || stdout != "" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if strings.Contains(stderr, feedToken) || strings.Contains(stderr, "calendar-feed/") {
		t.Fatalf("stderr leaked the feed URL: %q", stderr)
	}
	if code := commandErrorCode(t, lastLine(stderr)); code != "interrupted" {
		t.Fatalf("code = %q, stderr=%q", code, stderr)
	}
}

func TestInterruptedBackgroundUploadHidesPresignedURL(t *testing.T) {
	started := make(chan struct{})
	storage := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Drain the upload so the server notices the client disconnect.
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer storage.Close()
	uploadURL := storage.URL + "/bucket/obj?X-Amz-Signature=presigned-sig"

	api, _ := bodyServer(t, http.StatusOK, "application/json", `{"key":"k1","uploadUrl":"`+uploadURL+`","version":"v7"}`)
	env := authedEnv(t, api)
	file := filepath.Join(t.TempDir(), "bg.png")
	if err := os.WriteFile(file, []byte("\x89PNG background bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, stdout, stderr := env.runInterrupted(started, "project", "create-background-upload", "--id", "p1", "--file", file)
	if status != 130 || stdout != "" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if strings.Contains(stderr, "presigned-sig") || strings.Contains(stderr, storage.URL) || strings.Contains(stderr, "/bucket/obj") {
		t.Fatalf("stderr leaked the presigned URL: %q", stderr)
	}
	if code := commandErrorCode(t, lastLine(stderr)); code != "interrupted" {
		t.Fatalf("code = %q, stderr=%q", code, stderr)
	}
}

func lastLine(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	return lines[len(lines)-1]
}

func TestGitlabUpdateIntegrationRedactsWebhookSecret(t *testing.T) {
	server, requests := bodyServer(t, http.StatusOK, "application/json", `{"id":"g1","webhookSecret":"hook-secret","n":12345678901234567890}`)
	env := authedEnv(t, server)
	env.setStdin(`{}`)
	status, stdout, stderr := env.run("gitlab", "update-integration", "--project-id", "p1", "--body-file", "-")
	if status != 0 || stdout != `{"id":"g1","n":12345678901234567890,"webhookSecret":"[REDACTED]"}`+"\n" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "hook-secret") {
		t.Fatal("webhook secret leaked")
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %+v", *requests)
	}
}

func TestProjectMoveRequiresYes(t *testing.T) {
	server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
	env := authedEnv(t, server)
	env.setStdin(`{"workspaceId":"w2"}`)
	status, stdout, stderr := env.run("project", "move", "--id", "p1", "--body-file", "-")
	if status != 2 || stdout != "" || !strings.Contains(stderr, "--yes") {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if len(*requests) != 0 {
		t.Fatalf("requests = %+v", *requests)
	}
}

func TestGitlabProviderRejectionMapsToExit5(t *testing.T) {
	const body = `{"message":"echoed glpat-synthetic-provider-token"}`
	for _, args := range [][]string{
		{"gitlab", "verify-access"},
		{"gitlab", "list-projects"},
	} {
		t.Run(args[1], func(t *testing.T) {
			server, _ := bodyServer(t, http.StatusUnauthorized, "application/json", body)
			env := authedEnv(t, server)
			env.setStdin(`{"accessToken":"glpat-synthetic-provider-token"}`)
			status, stdout, stderr := env.run(append(args, "--body-file", "-")...)
			if status != 5 || stdout != "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if code := commandErrorCode(t, lastLine(stderr)); code != "provider_rejected" {
				t.Fatalf("code = %q, stderr=%q", code, stderr)
			}
			if strings.Contains(stderr, "glpat-synthetic-provider-token") || strings.Contains(stderr, "echoed") {
				t.Fatalf("stderr echoed the body: %q", stderr)
			}
			if !strings.Contains(stderr, "the Kaneo credential was accepted") {
				t.Fatalf("stderr = %q", stderr)
			}
		})
	}

	for _, args := range [][]string{
		{"gitlab", "create-integration", "--project-id", "p1", "--body-file", "-"},
		{"gitea", "verify-access", "--body-file", "-"},
		{"instance", "get-status"},
	} {
		server, _ := bodyServer(t, http.StatusUnauthorized, "application/json", `{"message":"Unauthorized"}`)
		env := authedEnv(t, server)
		env.setStdin(`{}`)
		status, _, stderr := env.run(args...)
		if status != 3 {
			t.Fatalf("%v: status=%d stderr=%q", args, status, stderr)
		}
		if code := commandErrorCode(t, lastLine(stderr)); code != "authentication_failed" {
			t.Fatalf("%v: code = %q", args, code)
		}
	}
}

func TestParamLengthCountsCharacters(t *testing.T) {
	server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
	env := authedEnv(t, server)
	if status, _, stderr := env.run("admin", "list-users", "--search", strings.Repeat("é", 150)); status != 0 {
		t.Fatalf("150 runes: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("admin", "list-users", "--search", strings.Repeat("é", 201)); status != 2 || !strings.Contains(stderr, "at most 200 characters") {
		t.Fatalf("201 runes: status=%d stderr=%q", status, stderr)
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %+v", *requests)
	}
}

func TestBoundedParamHelpShowsRange(t *testing.T) {
	env := newTestEnv(t)
	status, stdout, stderr := env.run("admin", "list-users", "--help")
	if status != 0 || !strings.Contains(stdout, "(integer 1-1000000)") || !strings.Contains(stdout, "(integer 1-100)") {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}
