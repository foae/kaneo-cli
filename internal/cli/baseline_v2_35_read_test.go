package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestV235ReadValidationBeforeNetwork(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	cases := []struct {
		name string
		args []string
	}{
		{"activity-limit-zero", []string{"activity", "list-task", "--task-id", "t", "--limit", "0"}},
		{"activity-limit-too-large", []string{"activity", "list-task", "--task-id", "t", "--limit", "101"}},
		{"activity-limit-fraction", []string{"activity", "list-task", "--task-id", "t", "--limit", "1.5"}},
		{"activity-workspace-empty", []string{"activity", "list-workspace", "--workspace-id="}},
		{"notification-workspace-empty", []string{"notification", "list", "--workspace-id="}},
		{"ticket-slug-empty", []string{"task", "get-by-ticket-id", "--ticket-id", "KAN-1", "--workspace-slug="}},
		{"ticket-slug-too-long", []string{"task", "get-by-ticket-id", "--ticket-id", "KAN-1", "--workspace-slug", strings.Repeat("s", 129)}},
		{"assigned-workspace-missing", []string{"task", "list-assigned"}},
		{"assigned-workspace-empty", []string{"task", "list-assigned", "--workspace-id="}},
		{"assigned-count-invalid", []string{"task", "list-assigned", "--workspace-id", "w", "--count-only", "FALSE"}},
		{"assigned-count-empty", []string{"task", "list-assigned", "--workspace-id", "w", "--count-only="}},
		{"admin-search-too-long", []string{"admin", "list-workspaces", "--search", strings.Repeat("s", 201)}},
		{"admin-page-zero", []string{"admin", "list-workspaces", "--page", "0"}},
		{"admin-page-too-large", []string{"admin", "list-workspaces", "--page", "1000001"}},
		{"admin-page-fraction", []string{"admin", "list-workspaces", "--page", "1.5"}},
		{"admin-limit-zero", []string{"admin", "list-workspaces", "--limit", "0"}},
		{"admin-limit-too-large", []string{"admin", "list-workspaces", "--limit", "101"}},
		{"sync-project-empty", []string{"integration-sync", "get-rules", "--project-id=", "--provider", "github"}},
		{"sync-provider-invalid", []string{"integration-sync", "get-rules", "--project-id", "p", "--provider", "bitbucket"}},
		{"review-project-empty", []string{"integration-sync", "review-resume", "--project-id=", "--provider", "gitea", "--link-id", "l"}},
		{"review-provider-invalid", []string{"integration-sync", "review-resume", "--project-id", "p", "--provider", "GitLab", "--link-id", "l"}},
		{"review-link-empty", []string{"integration-sync", "review-resume", "--project-id", "p", "--provider", "gitlab", "--link-id="}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			status, stdout, stderr := env.run(test.args...)
			if status != 2 || stdout != "" || stderr == "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if requests.Load() != 0 {
				t.Fatalf("invalid input reached network: %d requests", requests.Load())
			}
		})
	}
}

func TestV235ReadOptionalQueriesAndBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		query string
	}{
		{"count-omitted", []string{"task", "list-assigned", "--workspace-id", "w"}, "workspaceId=w"},
		{"count-false", []string{"task", "list-assigned", "--workspace-id", "w", "--count-only", "false"}, "countOnly=false&workspaceId=w"},
		{"count-true", []string{"task", "list-assigned", "--workspace-id", "w", "--count-only", "true"}, "countOnly=true&workspaceId=w"},
		{"activity-unbounded", []string{"activity", "list-task", "--task-id", "t"}, ""},
		{"activity-min", []string{"activity", "list-task", "--task-id", "t", "--limit", "1"}, "limit=1"},
		{"activity-max", []string{"activity", "list-task", "--task-id", "t", "--limit", "100"}, "limit=100"},
		{"admin-defaults-omitted", []string{"admin", "list-workspaces"}, ""},
		{"admin-min", []string{"admin", "list-workspaces", "--page", "1", "--limit", "1"}, "limit=1&page=1"},
		{"admin-max", []string{"admin", "list-workspaces", "--page", "1000000", "--limit", "100", "--search", strings.Repeat("s", 200)}, "limit=100&page=1000000&search=" + strings.Repeat("s", 200)},
		{"notification-unscoped", []string{"notification", "list"}, ""},
		{"notification-scoped", []string{"notification", "list", "--workspace-id", "w"}, "workspaceId=w"},
		{"ticket-compatible-scopes", []string{"task", "get-by-ticket-id", "--ticket-id", "KAN-1", "--workspace-id", "w", "--workspace-slug", strings.Repeat("s", 128), "--project-id", "p"}, "projectId=p&workspaceId=w&workspaceSlug=" + strings.Repeat("s", 128)},
		{"members-project-omitted", []string{"workspace", "list-members", "--workspace-id", "w"}, ""},
		{"members-project-empty-allowed", []string{"workspace", "list-members", "--workspace-id", "w", "--project-id="}, "projectId="},
		{"invitations-org-omitted", []string{"org", "list-invitations"}, ""},
		{"invitations-org-empty-allowed", []string{"org", "list-invitations", "--organization-id="}, "organizationId="},
		{"sync-cursor-omitted", []string{"integration-sync", "get-rules", "--project-id", "p", "--provider", "github"}, ""},
		{"sync-cursor-empty-allowed", []string{"integration-sync", "get-rules", "--project-id", "p", "--provider", "gitea", "--after="}, "after="},
		{"sync-cursor-explicit", []string{"integration-sync", "get-rules", "--project-id", "p", "--provider", "gitlab", "--after", "next cursor"}, "after=next+cursor"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.RawQuery != test.query {
					t.Errorf("query=%q want %q", r.URL.RawQuery, test.query)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			if status, _, stderr := env.run(test.args...); status != 0 {
				t.Fatalf("status=%d stderr=%q", status, stderr)
			}
			if requests.Load() != 1 {
				t.Fatalf("requests=%d want 1", requests.Load())
			}
		})
	}
}

func TestV235ReadsDoNotAutomaticallyPage(t *testing.T) {
	cases := []struct {
		name string
		args []string
		body string
	}{
		{"assigned-truncated", []string{"task", "list-assigned", "--workspace-id", "w"}, `{"tasks":[{"id":"first-task"}],"total":101}`},
		{"admin-more-pages", []string{"admin", "list-workspaces", "--page", "1", "--limit", "1"}, `{"workspaces":[{"id":"first-workspace"}],"total":3}`},
		{"sync-next-cursor", []string{"integration-sync", "get-rules", "--project-id", "p", "--provider", "github", "--after", "previous"}, `{"isActive":false,"pausedTasks":[{"id":"paused-task","linkId":"link","eligible":false}],"pausedNextCursor":"next","total":200,"previewToken":"integrity-token"}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			status, stdout, stderr := env.run(test.args...)
			if status != 0 {
				t.Fatalf("status=%d stderr=%q", status, stderr)
			}
			if requests.Load() != 1 {
				t.Fatalf("automatically paged: %d requests", requests.Load())
			}
			var got, want any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(test.body), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("response changed: got %s want %s", stdout, test.body)
			}
		})
	}
}

func TestV235SyncReviewFailuresAreNotRetried(t *testing.T) {
	for _, code := range []int{http.StatusConflict, http.StatusBadGateway} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				http.Error(w, "comparison changed or external issue unavailable", code)
			}))
			defer server.Close()
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			status, stdout, stderr := env.run("integration-sync", "review-resume", "--project-id", "p", "--provider", "gitlab", "--link-id", "l")
			if status == 0 || stdout != "" || stderr == "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if requests.Load() != 1 {
				t.Fatalf("review retried: %d requests", requests.Load())
			}
		})
	}
}
