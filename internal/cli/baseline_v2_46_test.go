package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	v246ConnectionBody = `{"workspaceId":"w1","connection":{"source":"trello","token":"synthetic-provider-token"}}`
	v246StartBody      = `{"workspaceId":"w1","connection":{"source":"trello","token":"synthetic-provider-token"},"projectIds":["board1"],"includeComments":false}`
	v246RunEnvelope    = `{"run":{"id":"r1","workspaceId":"w1","source":"trello","status":"running","includeComments":false,"message":"Starting","error":null,"projects":[{"sourceId":"board1","name":"Board","status":"pending","projectId":null,"projectSlug":null,"counts":{"columns":0,"tasks":0,"labels":0,"comments":0,"assignees":0,"relations":0},"warnings":[],"error":null}],"createdAt":"2026-10-13T00:00:00Z","finishedAt":null}}`
)

type v246Fixture struct {
	name     string
	args     []string
	method   string
	path     string
	query    string
	body     string
	response string
	status   int
	errors   []int
}

func v246Fixtures() []v246Fixture {
	return []v246Fixture{
		{"workspace-list", []string{"workspace", "list"}, "GET", "/workspace", "", "", `[{"id":"w1","name":"Workspace","slug":"workspace","logo":null,"description":null,"createdAt":"2026-10-13T00:00:00Z","role":"owner,viewer"}]`, 200, []int{401, 403, 429}},
		{"workspace-get", []string{"workspace", "get", "--workspace-id", "w1"}, "GET", "/workspace/w1", "", "", `{"id":"w1","name":"Workspace","slug":"workspace","logo":null,"description":null,"createdAt":"2026-10-13T00:00:00Z","role":null}`, 200, []int{401, 403, 404, 429}},
		{"workspace-capabilities", []string{"workspace", "get-my-capabilities", "--workspace-id", "w1"}, "GET", "/workspace/w1/capabilities", "", "", `{"manageProjects":false,"createProjects":true,"updateProjects":false,"shareProjects":false,"deleteProjects":false,"updateTasks":false,"createTasks":true,"deleteTasks":false,"assignTasks":false,"createLabels":false,"updateLabels":false,"deleteLabels":false,"manageWorkspace":false,"manageSettings":false,"deleteWorkspace":false,"inviteUsers":false,"manageTeam":false,"updateMembers":false,"removeMembers":false}`, 200, []int{401, 404, 429}},
		{"source-projects", []string{"import", "list-source-projects"}, "POST", "/import/projects", "", v246ConnectionBody, `{"projects":[{"id":"board1","name":"Board","detail":"Source workspace"}]}`, 200, []int{400, 401, 402, 403, 404, 429, 502}},
		{"start-run", []string{"import", "start-run"}, "POST", "/import/runs", "", v246StartBody, v246RunEnvelope, 202, []int{400, 401, 402, 403, 404, 409, 429, 502, 503}},
		{"list-runs", []string{"import", "list-runs", "--workspace-id", "w1"}, "GET", "/import/runs/w1", "", "", `{"runs":[` + strings.TrimSuffix(strings.TrimPrefix(v246RunEnvelope, `{"run":`), `}`) + `]}`, 200, []int{401, 403, 404, 429}},
		{"get-run", []string{"import", "get-run", "--workspace-id", "w1", "--run-id", "r1"}, "GET", "/import/runs/w1/r1", "", "", v246RunEnvelope, 200, []int{401, 403, 404, 429}},
		{"trello-authorize-url", []string{"import", "get-trello-authorize-url", "--workspace-id", "w1"}, "GET", "/import/trello/authorize-url/w1", "", "", `{"authorizeUrl":"https://trello.com/1/authorize?key=public-app-key&scope=read&response_type=token"}`, 200, []int{401, 403, 404, 429}},
		{"notification-unread", []string{"notification", "mark-unread", "--id", "n1"}, "PATCH", "/notification/n1/unread", "", "", `{"id":"n1","userId":"u1","title":null,"content":null,"type":"task_comment","isRead":false,"resourceId":"t1","resourceType":"task","createdAt":"2026-10-13T00:00:00Z","updatedAt":"2026-10-13T00:00:00Z"}`, 200, []int{401, 404, 429}},
		{"notification-delete", []string{"notification", "delete", "--id", "n1", "--yes"}, "DELETE", "/notification/n1", "", "", `{"success":true}`, 200, []int{401, 404, 429}},
		{"notification-clear-read", []string{"notification", "clear-read", "--workspace-id", "w1", "--yes"}, "DELETE", "/notification/clear-read", "workspaceId=w1", "", `{"success":true}`, 200, []int{401, 429}},
		{"custom-field-update", []string{"custom-field", "update", "--id", "f1"}, "PUT", "/custom-field/f1", "", `{"required":false,"defaultValue":null,"options":["Later","Now"]}`, `{"id":"f1","projectId":"p1","name":"Choice","type":"dropdown","required":false,"defaultValue":null,"options":["Later","Now"],"position":0,"createdAt":"2026-10-13T00:00:00Z","updatedAt":"2026-10-13T00:00:00Z"}`, 200, []int{400, 401, 403, 404, 429}},
	}
}

func v246RunFixture(t *testing.T, fixture v246Fixture, code int, response string, authenticated bool) (int, string, string, int64) {
	t.Helper()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != fixture.method || r.URL.Path != "/api"+fixture.path || r.URL.RawQuery != fixture.query {
			t.Errorf("request=%s %s want %s /api%s?%s", r.Method, r.URL, fixture.method, fixture.path, fixture.query)
		}
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Errorf("missing configured bearer credential")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != fixture.body {
			t.Errorf("body changed (read error=%v)", err)
		}
		if fixture.body != "" && r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type=%q", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	if authenticated {
		env.env["KANEO_TOKEN"] = "synthetic"
	}
	args := append([]string{}, fixture.args...)
	if fixture.body != "" {
		env.setStdin(fixture.body)
		args = append(args, "--body-file", "-")
	}
	status, stdout, stderr := env.run(args...)
	return status, stdout, stderr, requests.Load()
}

func v246AssertJSON(t *testing.T, stdout, expected string) {
	t.Helper()
	var got, want any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON output changed: got %s want %s", stdout, expected)
	}
}

func TestV246APIFailuresDoNotRetryOrPrintSuccess(t *testing.T) {
	for _, fixture := range v246Fixtures() {
		for _, code := range fixture.errors {
			t.Run(fixture.name+"/"+strconv.Itoa(code), func(t *testing.T) {
				status, stdout, stderr, requests := v246RunFixture(t, fixture, code, `{"code":"fixture_failure","message":"Request rejected"}`, true)
				want := 5
				if code == 401 || code == 403 {
					want = 3
				}
				if status != want || stdout != "" || !strings.Contains(stderr, `"error"`) || requests != 1 {
					t.Fatalf("status=%d want=%d stdout=%q stderr=%q requests=%d", status, want, stdout, stderr, requests)
				}
			})
		}
	}
}

func TestV246ValidationAndConfirmationBeforeNetwork(t *testing.T) {
	cases := [][]string{
		{"workspace", "get"},
		{"workspace", "get", "--workspace-id="},
		{"workspace", "get-my-capabilities"},
		{"import", "list-runs"},
		{"import", "get-run", "--workspace-id", "w1"},
		{"import", "get-trello-authorize-url"},
		{"import", "list-source-projects"},
		{"import", "start-run"},
		{"import", "start-run", "--workspace-id", "w1"},
		{"import", "list-source-projects", "--project-id", "p1"},
		{"notification", "mark-unread"},
		{"notification", "delete", "--id", "n1"},
		{"notification", "clear-read"},
		{"notification", "clear-read", "--yes", "--workspace-id="},
		{"custom-field", "update", "--id", "f1"},
		{"custom-field", "update", "--body-file", "-"},
		{"github", "list-repositories", "--project-id", "p1", "--search", strings.Repeat("s", 101)},
		{"github", "list-repositories", "--project-id", "p1", "--search", strings.Repeat("界", 101)},
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	for _, args := range cases {
		t.Run(strings.Join(args[:2], "-")+strconv.Itoa(len(args))+strings.Join(args[2:], ""), func(t *testing.T) {
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			env.setStdin(`{}`)
			status, stdout, stderr := env.run(args...)
			if status != 2 || stdout != "" || stderr == "" || requests.Load() != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%d", status, stdout, stderr, requests.Load())
			}
		})
	}
}

func TestV246GitHubSearchPaginationIsExplicit(t *testing.T) {
	response := `{"repositories":[],"nextPage":{"installationPage":2,"repositoryPage":3}}`
	for _, query := range []struct {
		args []string
		wire string
	}{
		{nil, ""},
		{[]string{"--search="}, "search="},
		{[]string{"--search", "Team / Board", "--installation-page", "2", "--repository-page", "3"}, "installationPage=2&repositoryPage=3&search=Team+%2F+Board"},
		{[]string{"--search", strings.Repeat("界", 100)}, "search=" + strings.Repeat("%E7%95%8C", 100)},
	} {
		fixture := v246Fixture{args: append([]string{"github", "list-repositories", "--project-id", "p1"}, query.args...), method: "GET", path: "/github-integration/repositories/p1", query: query.wire}
		status, stdout, stderr, requests := v246RunFixture(t, fixture, 200, response, true)
		if status != 0 || requests != 1 {
			t.Fatalf("status=%d stderr=%q requests=%d", status, stderr, requests)
		}
		v246AssertJSON(t, stdout, response)
	}
}

func TestV246AcceptedBackgroundImportDiffersFromIncompleteMutations(t *testing.T) {
	for _, test := range []struct {
		fixture v246Fixture
		want    int
	}{
		{v246Fixture{args: []string{"import", "start-run"}, method: "POST", path: "/import/runs", body: v246StartBody, response: v246RunEnvelope}, 0},
		{v246Fixture{args: []string{"github", "import-issues"}, method: "POST", path: "/github-integration/import-issues", body: `{"projectId":"p1"}`, response: `{"runId":"r1","pending":true,"imported":1,"updated":0,"skipped":0}`}, 5},
		{v246Fixture{args: []string{"label", "delete", "--id", "l1", "--yes"}, method: "DELETE", path: "/label/l1", response: `{"id":"l1","pendingDeletion":true}`}, 5},
	} {
		status, stdout, stderr, requests := v246RunFixture(t, test.fixture, http.StatusAccepted, test.fixture.response, true)
		if status != test.want || requests != 1 {
			t.Fatalf("status=%d want=%d stderr=%q requests=%d", status, test.want, stderr, requests)
		}
		v246AssertJSON(t, stdout, test.fixture.response)
		if (test.want == 5) != strings.Contains(stderr, `"code":"incomplete"`) {
			t.Fatalf("unexpected continuation diagnostic: %q", stderr)
		}
	}
}
