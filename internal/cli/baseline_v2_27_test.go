package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// recordingServer answers every request with the given status and body and
// records each request line and Authorization header.
type recordedRequest struct {
	method string
	target string
	auth   string
}

func recordingServer(t *testing.T, status int, body string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		requests = append(requests, recordedRequest{r.Method, target, r.Header.Get("Authorization")})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestNewReadCommandsHitDocumentedPaths(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		target string
		public bool
	}{
		{"public-board", []string{"project", "get-public", "--id", "p1", "--page", "2", "--related-page", "3", "--sort-by", "number"},
			"/api/public-project/p1?page=2&relatedPage=3&sortBy=number", true},
		{"public-description", []string{"project", "get-public-description", "--id", "p1", "--offset", "10", "--version", "4"},
			"/api/public-project/p1/description?offset=10&version=4", true},
		{"public-task-description", []string{"project", "get-public-task-description", "--id", "p1", "--task-id", "t1", "--offset", "0"},
			"/api/public-project/p1/task/t1/description?offset=0", true},
		{"description-matches", []string{"task", "find-description-matches", "--project-id", "p1", "--query", "needle", "--after", "c1"},
			"/api/task/description-matches/p1?after=c1&query=needle", false},
		{"task-description", []string{"task", "get-description", "--id", "t1", "--version", "7"},
			"/api/task/t1/description?version=7", false},
		{"task-list-related-page", []string{"task", "list", "--project-id", "p1", "--related-page", "2"},
			"/api/task/tasks/p1?relatedPage=2", false},
		{"github-repositories", []string{"github", "list-repositories", "--project-id", "p1", "--installation-page", "2", "--repository-page", "3"},
			"/api/github-integration/repositories/p1?installationPage=2&repositoryPage=3", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := recordingServer(t, http.StatusOK, `{"ok":true}`)
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			status, stdout, stderr := env.run(tt.args...)
			if status != 0 || stdout != "{\"ok\":true}\n" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if len(*requests) != 1 || (*requests)[0].method != "GET" || (*requests)[0].target != tt.target {
				t.Fatalf("requests = %+v, want GET %s", *requests, tt.target)
			}
			if auth := (*requests)[0].auth; tt.public && auth != "" {
				t.Fatalf("public read sent Authorization %q", auth)
			} else if !tt.public && auth == "" {
				t.Fatalf("authenticated read sent no Authorization")
			}
		})
	}
}

func TestNewReadParametersRejectInvalidValuesBeforeRequest(t *testing.T) {
	for _, args := range [][]string{
		{"github", "list-repositories", "--project-id", "p1", "--repository-page", "0"},
		{"github", "list-repositories", "--project-id", "p1", "--installation-page", "1234567"},
		{"task", "list", "--project-id", "p1", "--related-page", "x"},
		{"task", "get-description", "--id", "t1", "--version", "12345678901"},
		{"task", "find-description-matches", "--project-id", "p1", "--query", ""},
		{"project", "get-public-description", "--id", "p1", "--offset", "-1"},
	} {
		server, requests := recordingServer(t, http.StatusOK, `{}`)
		env := newTestEnv(t)
		env.setAPIURL(server.URL + "/api")
		env.env["KANEO_TOKEN"] = "synthetic"
		if status, stdout, stderr := env.run(args...); status != 2 || stdout != "" {
			t.Fatalf("%v: status=%d stdout=%q stderr=%q", args, status, stdout, stderr)
		}
		if len(*requests) != 0 {
			t.Fatalf("%v: sent %+v", args, *requests)
		}
	}
}

func TestIncompleteMutationsExitFiveWithoutRetry(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		method     string
		status     int
		body       string
		wantStatus int
	}{
		{"label-accepted", []string{"label", "delete", "--id", "l1", "--yes"}, "DELETE", http.StatusAccepted,
			`{"id":"l1","name":"bug","color":"#ff0000","createdAt":"2026-09-01T00:00:00Z","updatedAt":"2026-09-01T00:00:00Z","deletionStartedAt":"2026-09-24T00:00:00Z","taskId":null,"workspaceId":"w1","pendingDeletion":true}`, 5},
		{"label-done", []string{"label", "delete", "--id", "l1", "--yes"}, "DELETE", http.StatusOK,
			`{"id":"l1"}`, 0},
		{"import-accepted", []string{"github", "import-issues"}, "POST", http.StatusAccepted,
			`{"runId":"r1","pending":true,"imported":5,"updated":0,"skipped":0}`, 5},
		{"import-pending-ok", []string{"github", "import-issues"}, "POST", http.StatusOK,
			`{"runId":"r1","pending":true,"imported":5,"updated":0,"skipped":0}`, 5},
		{"import-done", []string{"github", "import-issues"}, "POST", http.StatusOK,
			`{"runId":"r1","pending":false,"imported":5,"updated":0,"skipped":0}`, 0},
		{"import-pending-non-boolean", []string{"github", "import-issues"}, "POST", http.StatusOK,
			`{"runId":"r1","pending":"yes","imported":5,"updated":0,"skipped":0}`, 0},
		{"import-pending-before-errors", []string{"github", "import-issues"}, "POST", http.StatusAccepted,
			`{"runId":"r1","pending":true,"imported":5,"skipped":1,"errors":["synthetic failure"]}`, 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := recordingServer(t, tt.status, tt.body)
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			args := tt.args
			if tt.method == "POST" {
				args = append(append([]string{}, args...), "--body-file", writeBodyFile(t, `{"projectId":"p1"}`))
			}
			status, stdout, stderr := env.run(args...)
			if status != tt.wantStatus || stdout != tt.body+"\n" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if tt.wantStatus == 5 && (!strings.Contains(stderr, `"code":"incomplete"`) || !strings.Contains(stderr, "still in progress")) {
				t.Fatalf("stderr = %q", stderr)
			}
			if tt.wantStatus == 0 && strings.Contains(stderr, "error") {
				t.Fatalf("stderr = %q", stderr)
			}
			if len(*requests) != 1 || (*requests)[0].method != tt.method {
				t.Fatalf("requests = %+v, want exactly one %s", *requests, tt.method)
			}
		})
	}
}

// sequencedBoardServer serves the project list and then board responses in
// request order, recording the page each board request asked for.
func sequencedBoardServer(t *testing.T, boards ...string) (*httptest.Server, *[]string) {
	t.Helper()
	var requests []string
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.EscapedPath(), "/api/project"):
			_, _ = w.Write([]byte(keyResolutionProjects))
		case strings.Contains(r.URL.EscapedPath(), "/api/task/tasks/"):
			requests = append(requests, "page="+r.URL.Query().Get("page"))
			if next >= len(boards) {
				t.Errorf("unexpected board request %d", next+1)
				_, _ = w.Write([]byte(`{"data":{"columns":[]}}`))
				return
			}
			_, _ = w.Write([]byte(boards[next]))
			next++
		default:
			requests = append(requests, r.URL.EscapedPath())
			_, _ = w.Write([]byte(`{"id":"t-150","title":"resolved task"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func boardWithTotal(page, total, totalPages int, numbers ...int) string {
	tasks := make([]string, 0, len(numbers))
	for _, number := range numbers {
		tasks = append(tasks, `{"id":"t-`+strconv.Itoa(number)+`","number":`+strconv.Itoa(number)+`}`)
	}
	return `{"data":{"columns":[{"tasks":[` + strings.Join(tasks, ",") + `]}]},"pagination":{"total":` + strconv.Itoa(total) +
		`,"page":` + strconv.Itoa(page) + `,"pageSize":100,"totalPages":` + strconv.Itoa(totalPages) + `}}`
}

func TestTaskKeyResolutionRestartsWhenBoardShrinks(t *testing.T) {
	// Task 150 slid from page 2 onto page 1 between the two page requests of
	// the first walk; the fresh walk finds it.
	server, requests := sequencedBoardServer(t,
		boardWithTotal(1, 200, 2, 1, 2),
		boardWithTotal(2, 199, 2, 152, 153),
		boardWithTotal(1, 199, 2, 1, 150, 151),
	)
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("task", "get", "--key", "KAN-150", "--workspace-id", "W")
	if status != 0 || !strings.Contains(stdout, "resolved task") {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	want := "page=1 page=2 page=1 /api/task/t-150"
	if got := strings.Join(*requests, " "); got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}

func TestTaskKeyResolutionReportsChangingBoard(t *testing.T) {
	server, requests := sequencedBoardServer(t,
		boardWithTotal(1, 200, 2, 1, 2),
		boardWithTotal(2, 199, 2, 152),
		boardWithTotal(1, 199, 2, 1, 2),
		boardWithTotal(2, 198, 2, 152),
	)
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("task", "get", "--key", "KAN-150", "--workspace-id", "W")
	if status != 1 || stdout != "" || !strings.Contains(stderr, "changed during the lookup") || strings.Contains(stderr, "no task") {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	want := "page=1 page=2 page=1 page=2"
	if got := strings.Join(*requests, " "); got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}

func TestTaskKeyResolutionReportsAbsenceOnStableSecondWalk(t *testing.T) {
	server, requests := sequencedBoardServer(t,
		boardWithTotal(1, 200, 2, 1, 2),
		boardWithTotal(2, 199, 2, 152),
		boardWithTotal(1, 199, 2, 1, 2),
		boardWithTotal(2, 199, 2, 152),
	)
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("task", "get", "--key", "KAN-150", "--workspace-id", "W")
	if status != 2 || stdout != "" || !strings.Contains(stderr, "no task") || strings.Contains(stderr, "changed during the lookup") {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	want := "page=1 page=2 page=1 page=2"
	if got := strings.Join(*requests, " "); got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}
