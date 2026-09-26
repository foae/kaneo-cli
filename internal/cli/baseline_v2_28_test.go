package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// bodyRequest is one request observed by bodyServer, including its raw body.
type bodyRequest struct {
	method string
	target string
	auth   string
	body   string
}

// bodyServer answers every request with the given status, content type and
// body, recording each request with its body.
func bodyServer(t *testing.T, status int, contentType, body string) (*httptest.Server, *[]bodyRequest) {
	t.Helper()
	var requests []bodyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		raw, _ := io.ReadAll(r.Body)
		requests = append(requests, bodyRequest{r.Method, target, r.Header.Get("Authorization"), string(raw)})
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func authedEnv(t *testing.T, server *httptest.Server) *testEnv {
	t.Helper()
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic-secret"
	return env
}

const feedToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestBaselineV228ReadsHitDocumentedPaths(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		target string
	}{
		{"calendar-feed-list", []string{"calendar-feed", "list", "--project-id", "p/1"}, "/api/calendar-feed/project/p%2F1"},
		{"gitlab-get", []string{"gitlab", "get-integration", "--project-id", "p1"}, "/api/gitlab-integration/project/p1"},
		{"user-get-current", []string{"user", "get-current"}, "/api/user/me"},
		{"admin-list-users", []string{"admin", "list-users", "--search", "a b", "--page", "2", "--limit", "100"}, "/api/admin/users?limit=100&page=2&search=a+b"},
		{"admin-list-users-bare", []string{"admin", "list-users"}, "/api/admin/users"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"ok":true}`
			if tt.name == "calendar-feed-list" {
				body = `[]`
			}
			server, requests := bodyServer(t, http.StatusOK, "application/json", body)
			env := authedEnv(t, server)
			status, stdout, stderr := env.run(tt.args...)
			if status != 0 || stdout == "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if len(*requests) != 1 || (*requests)[0].method != "GET" || (*requests)[0].target != tt.target {
				t.Fatalf("requests = %+v, want GET %s", *requests, tt.target)
			}
			if (*requests)[0].auth != "Bearer synthetic-secret" {
				t.Fatalf("authenticated read sent Authorization %q", (*requests)[0].auth)
			}
		})
	}
}

func TestBaselineV228WritesHitDocumentedPaths(t *testing.T) {
	const payload = `{"n":12345678901234567890,"unknown":{"x":[1,2]}}`
	for _, tt := range []struct {
		name   string
		args   []string
		method string
		target string
		body   bool
	}{
		{"calendar-feed-create", []string{"calendar-feed", "create", "--project-id", "p1"}, "POST", "/api/calendar-feed/project/p1", true},
		{"calendar-feed-revoke", []string{"calendar-feed", "revoke", "--project-id", "p1", "--id", "f/1", "--yes"}, "DELETE", "/api/calendar-feed/project/p1/f%2F1", false},
		{"project-move", []string{"project", "move", "--id", "p1"}, "PUT", "/api/project/p1/move", true},
		{"project-delete-background", []string{"project", "delete-background", "--id", "p1", "--yes"}, "DELETE", "/api/project/p1/background", false},
		{"project-finalize-background", []string{"project", "finalize-background-upload", "--id", "p1"}, "POST", "/api/project/p1/background-upload/finalize", true},
		{"task-duplicate", []string{"task", "duplicate", "--id", "t1"}, "POST", "/api/task/duplicate/t1", true},
		{"gitlab-list-projects", []string{"gitlab", "list-projects"}, "POST", "/api/gitlab-integration/projects", true},
		{"gitlab-verify", []string{"gitlab", "verify-access"}, "POST", "/api/gitlab-integration/verify", true},
		{"gitlab-create", []string{"gitlab", "create-integration", "--project-id", "p1"}, "POST", "/api/gitlab-integration/project/p1", true},
		{"gitlab-update", []string{"gitlab", "update-integration", "--project-id", "p1"}, "PATCH", "/api/gitlab-integration/project/p1", true},
		{"gitlab-delete", []string{"gitlab", "delete-integration", "--project-id", "p1", "--yes"}, "DELETE", "/api/gitlab-integration/project/p1", false},
		{"gitlab-import", []string{"gitlab", "import-issues"}, "POST", "/api/gitlab-integration/import-issues", true},
		{"external-link-create", []string{"external-link", "create", "--task-id", "t1"}, "POST", "/api/external-link/task/t1", true},
		{"external-link-delete", []string{"external-link", "delete", "--task-id", "t1", "--id", "l1", "--yes"}, "DELETE", "/api/external-link/task/t1/l1", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusOK, "application/json", `{"imported":1,"updated":0,"skipped":0}`)
			env := authedEnv(t, server)
			args := tt.args
			if tt.body {
				env.setStdin(payload)
				args = append(append([]string{}, args...), "--body-file", "-")
			}
			status, stdout, stderr := env.run(args...)
			if status != 0 || stdout != "{\"imported\":1,\"updated\":0,\"skipped\":0}\n" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if len(*requests) != 1 {
				t.Fatalf("requests = %+v", *requests)
			}
			got := (*requests)[0]
			if got.method != tt.method || got.target != tt.target {
				t.Fatalf("request = %s %s, want %s %s", got.method, got.target, tt.method, tt.target)
			}
			if got.auth != "Bearer synthetic-secret" {
				t.Fatalf("Authorization = %q", got.auth)
			}
			wantBody := ""
			if tt.body {
				wantBody = payload
			}
			if got.body != wantBody {
				t.Fatalf("body = %q, want %q", got.body, wantBody)
			}
		})
	}
}

func TestBaselineV228DuplicateAcceptsEmptyObject(t *testing.T) {
	server, requests := bodyServer(t, http.StatusOK, "application/json", `{"id":"t2"}`)
	env := authedEnv(t, server)
	env.setStdin(`{}`)
	if status, _, stderr := env.run("task", "duplicate", "--id", "t1", "--body-file", "-"); status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if len(*requests) != 1 || (*requests)[0].body != `{}` {
		t.Fatalf("requests = %+v", *requests)
	}
}

func TestBaselineV228DestructiveRequireYes(t *testing.T) {
	for _, args := range [][]string{
		{"calendar-feed", "revoke", "--project-id", "p1", "--id", "f1"},
		{"project", "delete-background", "--id", "p1"},
		{"gitlab", "delete-integration", "--project-id", "p1"},
		{"external-link", "delete", "--task-id", "t1", "--id", "l1"},
	} {
		server, requests := bodyServer(t, http.StatusNoContent, "", "")
		env := authedEnv(t, server)
		if status, stdout, stderr := env.run(args...); status != 2 || stdout != "" || !strings.Contains(stderr, "--yes") {
			t.Fatalf("%v: status=%d stdout=%q stderr=%q", args, status, stdout, stderr)
		}
		if len(*requests) != 0 {
			t.Fatalf("%v: sent %+v", args, *requests)
		}
		if status, stdout, stderr := env.run(append(args, "--yes")...); status != 0 || stdout != "" {
			t.Fatalf("%v --yes: status=%d stdout=%q stderr=%q", args, status, stdout, stderr)
		}
		if len(*requests) != 1 || (*requests)[0].method != "DELETE" {
			t.Fatalf("%v --yes: requests = %+v", args, *requests)
		}
	}
}

func TestBaselineV228BoundedParamsRejectBeforeRequest(t *testing.T) {
	for _, args := range [][]string{
		{"task", "list", "--project-id", "p1", "--page", "0"},
		{"task", "list", "--project-id", "p1", "--page", "1000001"},
		{"task", "list", "--project-id", "p1", "--related-page", "0"},
		{"task", "list", "--project-id", "p1", "--limit", "101"},
		{"task", "list", "--project-id", "p1", "--limit", "0"},
		{"task", "list", "--project-id", "p1", "--page", "99999999999999999999"},
		{"task", "list", "--project-id", "p1", "--page", "1a"},
		{"task", "list", "--project-id", "p1", "--page", "+1"},
		{"project", "get-public", "--id", "p1", "--limit", "101"},
		{"project", "get-public", "--id", "p1", "--page", "0"},
		{"project", "get-public-description", "--id", "p1", "--offset", "2000000001"},
		{"project", "get-public-task-description", "--id", "p1", "--task-id", "t1", "--offset", "99999999999999999999"},
		{"task", "get-description", "--id", "t1", "--offset", "x"},
		{"admin", "list-users", "--page", "0"},
		{"admin", "list-users", "--limit", "101"},
		{"admin", "list-users", "--search", strings.Repeat("a", 201)},
	} {
		server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
		env := authedEnv(t, server)
		status, stdout, stderr := env.run(args...)
		if status != 2 || stdout != "" {
			t.Fatalf("%v: status=%d stdout=%q stderr=%q", args, status, stdout, stderr)
		}
		if len(*requests) != 0 {
			t.Fatalf("%v: sent %+v", args, *requests)
		}
	}

	server, _ := bodyServer(t, http.StatusOK, "application/json", `{}`)
	env := authedEnv(t, server)
	_, _, stderr := env.run("task", "list", "--project-id", "p1", "--page", "0")
	if !strings.Contains(stderr, "--page must be an integer between 1 and 1000000") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestBaselineV228BoundedParamsAcceptBoundaries(t *testing.T) {
	for _, tt := range []struct {
		args   []string
		target string
	}{
		{[]string{"task", "list", "--project-id", "p1", "--page", "1", "--related-page", "1000000", "--limit", "100"},
			"/api/task/tasks/p1?limit=100&page=1&relatedPage=1000000"},
		{[]string{"project", "get-public", "--id", "p1", "--page", "1000000", "--limit", "1"},
			"/api/public-project/p1?limit=1&page=1000000"},
		{[]string{"project", "get-public-description", "--id", "p1", "--offset", "0"},
			"/api/public-project/p1/description?offset=0"},
		{[]string{"project", "get-public-task-description", "--id", "p1", "--task-id", "t1", "--offset", "2000000000"},
			"/api/public-project/p1/task/t1/description?offset=2000000000"},
		{[]string{"task", "get-description", "--id", "t1", "--offset", "2000000000"},
			"/api/task/t1/description?offset=2000000000"},
		{[]string{"admin", "list-users", "--page", "1000000", "--limit", "1", "--search", strings.Repeat("a", 200)},
			"/api/admin/users?limit=1&page=1000000&search=" + strings.Repeat("a", 200)},
	} {
		server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
		env := authedEnv(t, server)
		if status, _, stderr := env.run(tt.args...); status != 0 {
			t.Fatalf("%v: status=%d stderr=%q", tt.args, status, stderr)
		}
		if len(*requests) != 1 || (*requests)[0].target != tt.target {
			t.Fatalf("%v: requests = %+v, want %s", tt.args, *requests, tt.target)
		}
	}
}

func TestCalendarFeedListRedactsTokens(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"redacts", `[{"id":"f1","token":"secret-one","n":12345678901234567890},{"token":"secret-two","token":"secret-dup","id":"f2"}]`,
			`[{"id":"f1","n":12345678901234567890,"token":"[REDACTED]"},{"id":"f2","token":"[REDACTED]"}]` + "\n"},
		{"null-body", `null`, "null\n"},
		{"empty-array", `[]`, "[]\n"},
		{"null-or-absent-token", `[{"id":"f1","token":null},{"id":"f2"}]`, `[{"id":"f1","token":null},{"id":"f2"}]` + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := bodyServer(t, http.StatusOK, "application/json", tt.body)
			env := authedEnv(t, server)
			status, stdout, stderr := env.run("calendar-feed", "list", "--project-id", "p1")
			if status != 0 || stdout != tt.want {
				t.Fatalf("status=%d stdout=%q want %q stderr=%q", status, stdout, tt.want, stderr)
			}
			if strings.Contains(stdout+stderr, "secret-") {
				t.Fatalf("token leaked: stdout=%q stderr=%q", stdout, stderr)
			}
		})
	}
	for _, body := range []string{
		`[{"token":"secret-one"},"secret-two"]`,
		`[{"token":"secret-one"},null]`,
		`{"token":"secret-one"}`,
		`"secret-one"`,
		`[{"token":"secret-one"}`,
	} {
		server, _ := bodyServer(t, http.StatusOK, "application/json", body)
		env := authedEnv(t, server)
		status, stdout, stderr := env.run("calendar-feed", "list", "--project-id", "p1")
		if status == 0 || stdout != "" || strings.Contains(stderr, "secret-") {
			t.Fatalf("%s: status=%d stdout=%q stderr=%q", body, status, stdout, stderr)
		}
	}
}

func TestCalendarFeedCreatePrintsTokenUnredacted(t *testing.T) {
	body := `{"id":"f1","token":"` + feedToken + `"}`
	server, _ := bodyServer(t, http.StatusOK, "application/json", body)
	env := authedEnv(t, server)
	env.setStdin(`{}`)
	status, stdout, stderr := env.run("calendar-feed", "create", "--project-id", "p1", "--body-file", "-")
	if status != 0 || stdout != body+"\n" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

func TestGitlabGetIntegrationRedactsWebhookSecret(t *testing.T) {
	server, _ := bodyServer(t, http.StatusOK, "application/json", `{"id":"g1","webhookSecret":"hook-secret","n":12345678901234567890}`)
	env := authedEnv(t, server)
	status, stdout, stderr := env.run("gitlab", "get-integration", "--project-id", "p1")
	if status != 0 || stdout != `{"id":"g1","n":12345678901234567890,"webhookSecret":"[REDACTED]"}`+"\n" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

func TestCalendarFeedDownload(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"
	server, requests := bodyServer(t, http.StatusOK, "text/calendar", ics)
	env := authedEnv(t, server)
	if err := env.keyring.Set("default", server.URL+"/api", "stored-secret"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "feed.ics")

	if status, stdout, stderr := env.run("calendar-feed", "download", "--token", feedToken, "--output", dest); status != 0 || stdout != "" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != ics {
		t.Fatalf("file = %q, err=%v", data, err)
	}
	if status, _, stderr := env.run("calendar-feed", "download", "--token", feedToken, "--output", dest); status != 2 || !strings.Contains(stderr, "--force") {
		t.Fatalf("overwrite: status=%d stderr=%q", status, stderr)
	}
	if status, stdout, stderr := env.run("calendar-feed", "download", "--token", feedToken, "--output", "-"); status != 0 || stdout != ics {
		t.Fatalf("stdout: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if len(*requests) != 2 {
		t.Fatalf("requests = %+v", *requests)
	}
	for _, request := range *requests {
		if request.method != "GET" || request.target != "/api/calendar-feed/"+feedToken+"/calendar.ics" {
			t.Fatalf("request = %+v", request)
		}
		if request.auth != "" {
			t.Fatalf("public feed download sent Authorization %q", request.auth)
		}
	}

	for _, token := range []string{"short", strings.ToUpper(feedToken), feedToken + "0"} {
		if status, _, stderr := env.run("calendar-feed", "download", "--token", token, "--output", "-"); status != 2 {
			t.Fatalf("%s: status=%d stderr=%q", token, status, stderr)
		}
	}
	if len(*requests) != 2 {
		t.Fatalf("invalid token issued requests: %+v", *requests)
	}
}

func TestCalendarFeedDownloadNotFoundHidesToken(t *testing.T) {
	server, _ := bodyServer(t, http.StatusNotFound, "text/plain", "Calendar feed not found")
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	status, stdout, stderr := env.run("calendar-feed", "download", "--token", feedToken, "--output", "-")
	if status == 0 || stdout != "" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if strings.Contains(stderr, feedToken) {
		t.Fatalf("stderr leaked the feed token: %q", stderr)
	}
}

func TestProjectDownloadBackground(t *testing.T) {
	image := "\x89PNG\x00\xff"
	server, requests := bodyServer(t, http.StatusOK, "image/png", image)
	env := authedEnv(t, server)
	dest := filepath.Join(t.TempDir(), "bg.png")
	if status, _, stderr := env.run("project", "download-background", "--id", "p1", "--output", dest); status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != image {
		t.Fatalf("file = %q err=%v", data, err)
	}
	if len(*requests) != 1 || (*requests)[0].target != "/api/project/p1/background" || (*requests)[0].auth != "Bearer synthetic-secret" {
		t.Fatalf("requests = %+v", *requests)
	}
}

func TestProjectCreateBackgroundUpload(t *testing.T) {
	image := []byte("\x89PNG background bytes")
	var storageBody, storageAuth, storageHeader string
	storageHits := 0
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageHits++
		raw, _ := io.ReadAll(r.Body)
		storageBody = string(raw)
		storageAuth = r.Header.Get("Authorization")
		storageHeader = r.Header.Get("X-Amz-Test")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(storage.Close)
	uploadURL := storage.URL + "/bucket/obj?X-Amz-Signature=presigned-sig"

	respond := func(body string) (*httptest.Server, *[]bodyRequest) {
		return bodyServer(t, http.StatusOK, "application/json", body)
	}
	file := filepath.Join(t.TempDir(), "bg.png")
	if err := os.WriteFile(file, image, 0o600); err != nil {
		t.Fatal(err)
	}

	api, requests := respond(`{"key":"k1","uploadUrl":"` + uploadURL + `","version":"v7","headers":{"X-Amz-Test":"h"}}`)
	env := authedEnv(t, api)
	status, stdout, stderr := env.run("project", "create-background-upload", "--id", "p1", "--file", file)
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	want := `{"key":"k1","contentType":"image/png","version":"v7","size":` + strconv.Itoa(len(image)) + "}\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if strings.Contains(stdout+stderr, "presigned-sig") || strings.Contains(stdout+stderr, storage.URL) {
		t.Fatalf("upload URL leaked: stdout=%q stderr=%q", stdout, stderr)
	}
	if storageBody != string(image) || storageAuth != "" || storageHeader != "h" {
		t.Fatalf("storage body=%q auth=%q header=%q", storageBody, storageAuth, storageHeader)
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %+v", *requests)
	}
	got := (*requests)[0]
	if got.method != "PUT" || got.target != "/api/project/p1/background-upload" || got.auth != "Bearer synthetic-secret" ||
		got.body != `{"contentType":"image/png","size":`+strconv.Itoa(len(image))+`}` {
		t.Fatalf("request = %+v", got)
	}

	for _, body := range []string{
		`{"uploadUrl":"` + uploadURL + `","version":"v7"}`,
		`{"key":"k1","uploadUrl":"` + uploadURL + `"}`,
		`{"key":"k1","version":"v7"}`,
	} {
		hitsBefore := storageHits
		api, _ := respond(body)
		env := authedEnv(t, api)
		status, stdout, stderr := env.run("project", "create-background-upload", "--id", "p1", "--file", file)
		if status == 0 || stdout != "" || strings.Contains(stderr, "presigned-sig") {
			t.Fatalf("%s: status=%d stdout=%q stderr=%q", body, status, stdout, stderr)
		}
		if storageHits != hitsBefore {
			t.Fatalf("%s: storage contacted despite incomplete response", body)
		}
	}

	empty := filepath.Join(t.TempDir(), "empty.png")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(t.TempDir(), "bg.bin")
	if err := os.WriteFile(unknown, image, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--id", "p1", "--file", empty},
		{"--id", "p1", "--file", unknown},
		{"--id", "..", "--file", file},
		{"--file", file},
		{"--id", "p1"},
	} {
		api, requests := respond(`{}`)
		env := authedEnv(t, api)
		status, stdout, stderr := env.run(append([]string{"project", "create-background-upload"}, args...)...)
		if status != 2 || stdout != "" {
			t.Fatalf("%v: status=%d stdout=%q stderr=%q", args, status, stdout, stderr)
		}
		if len(*requests) != 0 {
			t.Fatalf("%v: sent %+v", args, *requests)
		}
	}
}

func TestGitlabImportIssuesPartialFailure(t *testing.T) {
	for _, tt := range []struct {
		body       string
		wantStatus int
	}{
		{`{"imported":1,"updated":0,"skipped":0,"errors":["issue 3 failed"]}`, 5},
		{`{"imported":1,"updated":0,"skipped":0,"errors":[]}`, 0},
		{`{"imported":1,"updated":0,"skipped":0}`, 0},
	} {
		server, requests := bodyServer(t, http.StatusOK, "application/json", tt.body)
		env := authedEnv(t, server)
		env.setStdin(`{"projectId":"p1"}`)
		status, stdout, stderr := env.run("gitlab", "import-issues", "--body-file", "-")
		if status != tt.wantStatus || stdout != tt.body+"\n" {
			t.Fatalf("%s: status=%d stdout=%q stderr=%q", tt.body, status, stdout, stderr)
		}
		if tt.wantStatus == 5 && !strings.Contains(stderr, "partial_failure") {
			t.Fatalf("stderr = %q", stderr)
		}
		if len(*requests) != 1 {
			t.Fatalf("requests = %+v", *requests)
		}
	}
}

func TestBaselineV228ErrorStatuses(t *testing.T) {
	t.Run("invite-email-failed", func(t *testing.T) {
		server, _ := bodyServer(t, http.StatusBadGateway, "application/json", `{"code":"INVITATION_EMAIL_FAILED","message":"Invitation email could not be sent"}`)
		env := authedEnv(t, server)
		env.setStdin(`{"email":"a@example.com","role":"member"}`)
		status, stdout, stderr := env.run("org", "invite-member", "--body-file", "-")
		if status != 5 || stdout != "" || !strings.Contains(stderr, "Invitation email could not be sent") {
			t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
		}
	})
	t.Run("gitea-verify-text-not-echoed", func(t *testing.T) {
		server, requests := bodyServer(t, http.StatusInternalServerError, "text/plain", "upstream said tok-leak")
		env := authedEnv(t, server)
		const body = `{"baseUrl":"https://gitea.example.com"}`
		env.setStdin(body)
		status, stdout, stderr := env.run("gitea", "verify-access", "--body-file", "-")
		if status != 5 || stdout != "" || strings.Contains(stderr, "tok-leak") {
			t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
		}
		if len(*requests) != 1 || (*requests)[0].body != body {
			t.Fatalf("requests = %+v", *requests)
		}
	})
}
