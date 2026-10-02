package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBaselineV230ReadViewsAndTicketLookup(t *testing.T) {
	const result = `{"id":"t1","title":"Keep description","description":null,"descriptionDeferred":true,"subtaskCounts":{"completed":1,"total":2}}`
	for _, tt := range []struct {
		name   string
		args   []string
		target string
	}{
		{"view-omitted", []string{"task", "get", "--id", "t1"}, "/api/task/t1"},
		{"view-board", []string{"task", "get", "--id", "t1", "--view", "board"}, "/api/task/t1?view=board"},
		{"view-detail", []string{"task", "get", "--id", "t1", "--view", "detail"}, "/api/task/t1?view=detail"},
		{"ticket", []string{"task", "get-by-ticket-id", "--ticket-id", "KAN-12"}, "/api/task/by-ticket-id/KAN-12"},
		{"ticket-scoped", []string{"task", "get-by-ticket-id", "--ticket-id", "KAN-12", "--workspace-id", "w1", "--project-id", "p1"}, "/api/task/by-ticket-id/KAN-12?projectId=p1&workspaceId=w1"},
		{"ticket-schema-only", []string{"task", "get-by-ticket-id", "--ticket-id", strings.Repeat("é", 128)}, "/api/task/by-ticket-id/" + strings.Repeat("%C3%A9", 128)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusOK, "application/json", result)
			status, stdout, stderr := authedEnv(t, server).run(tt.args...)
			if status != 0 || stdout != result+"\n" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if len(*requests) != 1 || (*requests)[0].method != "GET" || (*requests)[0].target != tt.target || (*requests)[0].auth != "Bearer synthetic-secret" {
				t.Fatalf("unexpected requests: %+v", *requests)
			}
		})
	}
}

func TestBaselineV230ParametersRejectBeforeNetwork(t *testing.T) {
	for _, args := range [][]string{
		{"task", "get-by-ticket-id"},
		{"task", "get-by-ticket-id", "--ticket-id", ""},
		{"task", "get-by-ticket-id", "--ticket-id", strings.Repeat("a", 129)},
		{"task", "get-by-ticket-id", "--ticket-id", "KAN-12", "--workspace-id", ""},
		{"task", "get-by-ticket-id", "--ticket-id", "KAN-12", "--project-id", ""},
		{"task", "get", "--id", "t1", "--view", "summary"},
		{"task", "get", "--id", "t1", "--view", ""},
		{"task", "finalize-staged-asset", "--body-file", "-"},
		{"task", "reorder"},
		{"task", "finalize-staged-asset", "--project-id", "p1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
			env := authedEnv(t, server)
			env.setStdin(`{}`)
			status, stdout, stderr := env.run(args...)
			if status != 2 || stdout != "" || len(*requests) != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%+v", status, stdout, stderr, *requests)
			}
		})
	}
}

func TestBaselineV230ReorderPreservesGenericBody(t *testing.T) {
	const payload = `{"projectId":"p1","expectedTasks":[{"id":"t1","position":null,"status":"todo"}],"tasks":[{"id":"t1","position":0,"status":"done"}],"unknown":{"n":12345678901234567890}}`
	const result = `[{"id":"t1","position":0,"status":"done"}]`
	server, requests := bodyServer(t, http.StatusOK, "application/json", result)
	env := authedEnv(t, server)
	env.setStdin(payload)
	status, stdout, stderr := env.run("task", "reorder", "--body-file", "-")
	if status != 0 || stdout != result+"\n" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if len(*requests) != 1 || (*requests)[0].method != "POST" || (*requests)[0].target != "/api/task/reorder" || (*requests)[0].body != payload || (*requests)[0].auth != "Bearer synthetic-secret" {
		t.Fatalf("unexpected requests: %+v", *requests)
	}
}

func TestBaselineV230FailuresNeverRetry(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		httpStatus int
		body       string
		wantExit   int
	}{
		{"ticket-missing", []string{"task", "get-by-ticket-id", "--ticket-id", "KAN-12"}, 404, "No accessible task has this ticket ID", 5},
		{"ticket-ambiguous", []string{"task", "get-by-ticket-id", "--ticket-id", "KAN-12"}, 409, "Ticket ID matches multiple accessible tasks", 5},
		{"ticket-auth", []string{"task", "get-by-ticket-id", "--ticket-id", "KAN-12"}, 401, "Missing or invalid credentials", 3},
		{"reorder-stale", []string{"task", "reorder", "--body-file", "-"}, 409, "Board changed or task moved; refresh before reordering", 5},
		{"reorder-auth", []string{"task", "reorder", "--body-file", "-"}, 401, "Missing or invalid credentials", 3},
		{"finalize-attached", []string{"task", "finalize-staged-asset", "--project-id", "p1", "--body-file", "-"}, 409, "Upload already attached", 5},
		{"finalize-auth", []string{"task", "finalize-staged-asset", "--project-id", "p1", "--body-file", "-"}, 401, "Missing or invalid credentials", 3},
		{"move-concurrent", []string{"task", "move", "--id", "t1", "--body-file", "-"}, 409, "Task or project moved concurrently; retry the move", 5},
		{"delete-concurrent", []string{"task", "delete", "--id", "t1", "--yes"}, 409, "Task changed projects; retry the operation", 5},
		{"bulk-concurrent", []string{"task", "bulk-update", "--body-file", "-", "--yes"}, 409, "Tasks changed projects; retry the operation", 5},
		{"github-import-permission", []string{"github", "import-issues", "--body-file", "-"}, 403, "Missing task:update permission", 3},
		{"gitea-import-permission", []string{"gitea", "import-issues", "--body-file", "-"}, 403, "Missing task:update permission", 3},
		{"gitlab-import-permission", []string{"gitlab", "import-issues", "--body-file", "-"}, 403, "Missing task:update permission", 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := bodyServer(t, tt.httpStatus, "text/plain", tt.body)
			env := authedEnv(t, server)
			env.setStdin(`{"projectId":"p1","expectedTasks":[{"id":"t1","position":null,"status":"todo"}],"tasks":[{"id":"t1","position":0}]}`)
			status, stdout, stderr := env.run(tt.args...)
			if status != tt.wantExit || stdout != "" || len(*requests) != 1 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%+v", status, stdout, stderr, *requests)
			}
			lines := strings.Split(strings.TrimSpace(stderr), "\n")
			var response errorResponse
			if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil || response.Error.Status != tt.httpStatus {
				t.Fatalf("diagnostic=%q error=%v", stderr, err)
			}
			wantCode := "conflict"
			switch tt.httpStatus {
			case 404:
				wantCode = "not_found"
			case 401:
				wantCode = "authentication_failed"
			case 403:
				wantCode = "authorization_failed"
			}
			if response.Error.Code != wantCode || (tt.httpStatus == 409 && !strings.Contains(stderr, tt.body)) {
				t.Fatalf("error=%+v stderr=%q", response.Error, stderr)
			}
			if (*requests)[0].auth != "Bearer synthetic-secret" {
				t.Fatal("missing bearer authentication")
			}
			if tt.name == "reorder-stale" && !strings.Contains((*requests)[0].body, `"position":null`) {
				t.Fatal("nullable expected position lost")
			}
		})
	}
}

func TestBaselineV230FinalizeCreateLifecycle(t *testing.T) {
	stage := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-secret" || r.Method != "POST" {
			t.Error("missing authenticated POST")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch stage {
		case 0:
			if r.URL.Path != "/api/task/draft-upload/p1/finalize" || string(raw) != `{"key":"draft/key","filename":"notes.png","contentType":"image/png","size":10,"surface":"description"}` {
				t.Errorf("unexpected finalize %s body=%s", r.URL.Path, raw)
			}
			_, _ = io.WriteString(w, `{"id":"asset1","url":"/api/asset/asset1"}`)
		case 1:
			var body struct {
				DraftAssetIDs []string `json:"draftAssetIds"`
				Description   string   `json:"description"`
			}
			if err := json.Unmarshal(raw, &body); err != nil || r.URL.Path != "/api/task/p1" || len(body.DraftAssetIDs) != 1 || body.DraftAssetIDs[0] != "asset1" || body.Description != "![notes](/api/asset/asset1)" {
				t.Errorf("unexpected create %s body=%s error=%v", r.URL.Path, raw, err)
			}
			_, _ = io.WriteString(w, `{"id":"t1","description":"![notes](/api/asset/asset1)"}`)
		default:
			t.Error("unexpected retry")
		}
		stage++
	}))
	t.Cleanup(server.Close)
	env := authedEnv(t, server)
	env.setStdin(`{"key":"draft/key","filename":"notes.png","contentType":"image/png","size":10,"surface":"description"}`)
	status, stdout, stderr := env.run("task", "finalize-staged-asset", "--project-id", "p1", "--body-file", "-")
	if status != 0 || stdout != "{\"id\":\"asset1\",\"url\":\"/api/asset/asset1\"}\n" {
		t.Fatalf("finalize status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	env.setStdin(`{"title":"Notes","description":"![notes](/api/asset/asset1)","priority":"low","status":"todo","draftAssetIds":["asset1"]}`)
	status, stdout, stderr = env.run("task", "create", "--project-id", "p1", "--body-file", "-")
	if status != 0 || stdout != "{\"id\":\"t1\",\"description\":\"![notes](/api/asset/asset1)\"}\n" || stage != 2 {
		t.Fatalf("create status=%d stdout=%q stderr=%q stage=%d", status, stdout, stderr, stage)
	}
}

func TestBaselineV230GetViewAfterKeyResolution(t *testing.T) {
	server, requests := keyResolutionServer(t, keyResolutionProjects, `{"data":{"columns":[{"tasks":[{"id":"t-1","number":12}]}]}}`)
	t.Cleanup(server.Close)
	env := authedEnv(t, server)
	status, stdout, stderr := env.run("task", "get", "--key", "KAN-12", "--workspace-id", "W", "--view", "board")
	want := []string{"/api/project?includeArchived=true&workspaceId=W", boardPage(1), "/api/task/t-1?view=board"}
	if status != 0 || !strings.Contains(stdout, "resolved task") || strings.Join(*requests, " ") != strings.Join(want, " ") {
		t.Fatalf("status=%d stdout=%q stderr=%q requests=%v", status, stdout, stderr, *requests)
	}
}
