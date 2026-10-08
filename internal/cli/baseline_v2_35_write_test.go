package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestBaselineV235WriteConfirmationBeforeSession(t *testing.T) {
	for _, args := range [][]string{
		{"admin", "remove-workspace-member", "--workspace-id", "w1", "--user-id", "u1"},
		{"admin", "transfer-workspace-ownership", "--workspace-id", "w1"},
		{"workspace", "update-member-project-access", "--workspace-id", "w1", "--user-id", "u1"},
		{"integration-sync", "resume", "--project-id", "p1", "--provider", "github", "--link-id", "l1"},
		{"notification", "clear-all", "--workspace-id", "w1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.app.configDir = func() (string, error) {
				t.Fatal("confirmation rejection reached session or credential setup")
				return "", nil
			}
			status, stdout, stderr := env.run(args...)
			if status != 2 || stdout != "" || !strings.Contains(stderr, "without --yes") || len(*requests) != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%+v", status, stdout, stderr, *requests)
			}
		})
	}
}

func TestBaselineV235WriteParametersRejectBeforeSession(t *testing.T) {
	for _, args := range [][]string{
		{"integration-sync", "preview-rules", "--project-id", "p1", "--provider", "bitbucket", "--body-file", "-"},
		{"integration-sync", "preview-rules", "--project-id", "p1", "--provider", "", "--body-file", "-"},
		{"integration-sync", "save-rules", "--project-id", "", "--provider", "github", "--body-file", "-"},
		{"integration-sync", "save-rules", "--provider", "github", "--body-file", "-"},
		{"integration-sync", "resume", "--project-id", "p1", "--provider", "gitlab", "--link-id", "", "--body-file", "-", "--yes"},
		{"integration-sync", "resume", "--project-id", "p1", "--provider", "gitea", "--body-file", "-", "--yes"},
		{"admin", "add-workspace-member", "--workspace-id", "", "--body-file", "-"},
		{"admin", "remove-workspace-member", "--workspace-id", "w1", "--user-id", "", "--yes"},
		{"admin", "update-workspace-member-role", "--workspace-id", "w1", "--user-id", "", "--body-file", "-"},
		{"workspace", "update-member-project-access", "--workspace-id", "w1", "--user-id", "", "--body-file", "-", "--yes"},
		{"notification", "clear-all", "--workspace-id", "", "--yes"},
		{"notification", "mark-all-read", "--workspace-id", ""},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.setStdin(`{}`)
			env.app.configDir = func() (string, error) {
				t.Fatal("invalid parameters reached session or credential setup")
				return "", nil
			}
			status, stdout, stderr := env.run(args...)
			if status != 2 || stdout != "" || !strings.Contains(stderr, "invalid_arguments") || len(*requests) != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%+v", status, stdout, stderr, *requests)
			}
		})
	}
}

func TestBaselineV235SyncFailuresDoNotRetry(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		payload    string
		method     string
		target     string
		httpStatus int
		message    string
	}{
		{"stale-rules", []string{"integration-sync", "save-rules", "--project-id", "p1", "--provider", "github", "--body-file", "-"}, `{"rules":{"outgoing":{"mode":"all"},"incoming":{"mode":"all"}},"previewToken":"` + strings.Repeat("a", 64) + `"}`, "PATCH", "/api/integration-sync/project/p1/github", http.StatusConflict, "Configuration, impact or comparison changed; review again"},
		{"stale-resume", []string{"integration-sync", "resume", "--project-id", "p1", "--provider", "gitea", "--link-id", "l1", "--body-file", "-", "--yes"}, `{"token":"` + strings.Repeat("b", 64) + `","source":"provider"}`, "POST", "/api/integration-sync/project/p1/gitea/links/l1/resume", http.StatusConflict, "Comparison changed or synchronization is busy; review or retry"},
		{"provider-unavailable", []string{"integration-sync", "resume", "--project-id", "p1", "--provider", "gitlab", "--link-id", "l1", "--body-file", "-", "--yes"}, `{"token":"` + strings.Repeat("c", 64) + `","source":"kaneo"}`, "POST", "/api/integration-sync/project/p1/gitlab/links/l1/resume", http.StatusBadGateway, "External issue unavailable; sync remains paused"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := bodyServer(t, tt.httpStatus, "text/plain", tt.message)
			env := authedEnv(t, server)
			env.setStdin(tt.payload)
			status, stdout, stderr := env.run(tt.args...)
			if status != 5 || stdout != "" || len(*requests) != 1 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%+v", status, stdout, stderr, *requests)
			}
			request := (*requests)[0]
			if request.method != tt.method || request.target != tt.target || request.body != tt.payload {
				t.Fatalf("unexpected mutation: %+v", request)
			}
			lines := strings.Split(strings.TrimSpace(stderr), "\n")
			var response errorResponse
			if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil || response.Error.Status != tt.httpStatus {
				t.Fatalf("diagnostic=%q error=%v", stderr, err)
			}
			if response.Error.Message != tt.message {
				t.Fatalf("message=%q want %q", response.Error.Message, tt.message)
			}
			if tt.httpStatus == http.StatusConflict && response.Error.Code != "conflict" {
				t.Fatalf("error=%+v stderr=%q", response.Error, stderr)
			}
		})
	}
}
