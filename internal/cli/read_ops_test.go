package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// inventoryOperations loads the pinned machine inventory used to derive the
// read spec table.
func inventoryOperations(t *testing.T) []inventoryOperation {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "operations.json"))
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	var doc struct {
		Operations []inventoryOperation `json:"operations"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse inventory: %v", err)
	}
	// The source inventory deliberately stays faithful to OpenAPI. Apply only
	// separately reviewed CLI parameters from provenance when checking coverage.
	raw, err = os.ReadFile(filepath.Join("..", "..", "api", "provenance.json"))
	if err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	var provenance struct {
		Supplements map[string]inventoryOperation `json:"supplements"`
	}
	if err := json.Unmarshal(raw, &provenance); err != nil {
		t.Fatalf("parse provenance: %v", err)
	}
	for i := range doc.Operations {
		for _, supplement := range provenance.Supplements {
			if supplement.OperationID == doc.Operations[i].OperationID {
				doc.Operations[i].Parameters = append(doc.Operations[i].Parameters, supplement.Parameters...)
			}
		}
	}
	return doc.Operations
}

type inventoryParameter struct {
	In       string `json:"in"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Schema   *struct {
		Pattern   string   `json:"pattern"`
		Enum      []string `json:"enum"`
		MinLength int      `json:"minLength"`
		MaxLength int      `json:"maxLength"`
	} `json:"schema"`
}

// numericSchemaPattern is the spec pattern the numeric flag enforces.
const numericSchemaPattern = `^\d+$`

// checkParamConstraints compares each CLI parameter's local validation with
// the documented schema, so a spec refresh cannot tighten or loosen a
// constraint without the CLI following.
func checkParamConstraints(t *testing.T, operationID string, op inventoryOperation, params []readParam) {
	t.Helper()
	documented := make(map[string]inventoryParameter, len(op.Parameters))
	for _, param := range op.Parameters {
		documented[param.In+":"+param.Name] = param
	}
	for _, param := range params {
		location := "path"
		if param.in == paramQuery {
			location = "query"
		}
		doc, ok := documented[location+":"+param.name]
		if !ok {
			continue
		}
		var pattern string
		var enum []string
		var minLen, maxLen int
		if doc.Schema != nil {
			pattern, enum, minLen, maxLen = doc.Schema.Pattern, doc.Schema.Enum, doc.Schema.MinLength, doc.Schema.MaxLength
		}
		gotPattern := param.pattern
		if param.numeric {
			if gotPattern != "" {
				t.Errorf("%s %s: numeric param also sets pattern %q", operationID, param.name, gotPattern)
			}
			gotPattern = numericSchemaPattern
		}
		if gotPattern != pattern {
			t.Errorf("%s %s: pattern = %q, documented %q", operationID, param.name, gotPattern, pattern)
		}
		if strings.Join(param.enum, ",") != strings.Join(enum, ",") {
			t.Errorf("%s %s: enum = %v, documented %v", operationID, param.name, param.enum, enum)
		}
		if param.minLen != minLen || param.maxLen != maxLen {
			t.Errorf("%s %s: length = [%d,%d], documented [%d,%d]", operationID, param.name, param.minLen, param.maxLen, minLen, maxLen)
		}
	}
}

func TestNavigationParamsMatchInventory(t *testing.T) {
	ops := inventoryOperations(t)
	for operationID, params := range map[string][]navigationParam{
		"getDeviceAuthorizationPage": deviceAuthorizationPageParams,
		"authorizeMcpOAuthClient":    mcpAuthorizationParams,
	} {
		var op *inventoryOperation
		for i := range ops {
			if ops[i].OperationID == operationID {
				op = &ops[i]
			}
		}
		if op == nil {
			t.Errorf("%s: operation not in inventory", operationID)
			continue
		}
		plain := make([]readParam, 0, len(params))
		for _, param := range params {
			plain = append(plain, param.readParam)
		}
		if len(op.Parameters) != len(plain) {
			t.Errorf("%s: %d params, %d documented", operationID, len(plain), len(op.Parameters))
		}
		for _, param := range op.Parameters {
			found := false
			for _, local := range plain {
				if local.name == param.Name {
					found = true
					if local.required != param.Required {
						t.Errorf("%s %s: required differs", operationID, param.Name)
					}
				}
			}
			if !found {
				t.Errorf("%s: documented param %s missing", operationID, param.Name)
			}
		}
		checkParamConstraints(t, operationID, *op, plain)
	}
}

type inventoryOperation struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	OperationID string `json:"operation_id"`
	Command     struct {
		Group  string `json:"group"`
		Action string `json:"action"`
	} `json:"command"`
	Parameters  []inventoryParameter `json:"parameters"`
	RequestBody *json.RawMessage     `json:"request_body"`
	Security    []map[string]any     `json:"security"`
	Responses   map[string]struct {
		Content map[string]json.RawMessage `json:"content"`
	} `json:"responses"`
}

// readSpecExempt lists GET operations that are intentionally not part of the
// generated read table: the three packet-1 commands and the two browser
// navigation endpoints deferred pending a deliberate browser contract.
var readSpecExempt = map[string]string{
	"getSession":                 "packet 1: auth get-session",
	"getConfig":                  "packet 1: config get",
	"getInstanceStatus":          "packet 1: instance get-status",
	"getDeviceAuthorizationPage": "dedicated browser URL handoff",
	"authorizeMcpOAuthClient":    "dedicated browser URL handoff",
}

func TestReadSpecsMatchInventory(t *testing.T) {
	ops := inventoryOperations(t)
	byID := make(map[string]inventoryOperation, len(ops))
	for _, op := range ops {
		byID[op.OperationID] = op
	}

	seen := make(map[string]bool)
	for _, spec := range readSpecs {
		op, ok := byID[spec.operationID]
		if !ok {
			t.Errorf("%s: operation not in inventory", spec.operationID)
			continue
		}
		seen[spec.operationID] = true
		if op.Method != "GET" {
			t.Errorf("%s: method = %s, want GET", spec.operationID, op.Method)
		}
		if op.Path != spec.path {
			t.Errorf("%s: path = %s, want %s", spec.operationID, op.Path, spec.path)
		}
		if op.Command.Group != spec.group || op.Command.Action != spec.action {
			t.Errorf("%s: command = %s %s, spec = %s %s", spec.operationID, op.Command.Group, op.Command.Action, spec.group, spec.action)
		}
		if wantPublic := len(op.Security) == 0; wantPublic != spec.public {
			t.Errorf("%s: public = %v, inventory security = %v", spec.operationID, spec.public, op.Security)
		}
		if len(op.Parameters) != len(spec.params) {
			t.Errorf("%s: %d params in spec, %d in inventory", spec.operationID, len(spec.params), len(op.Parameters))
		}
		byName := make(map[string]bool)
		for _, param := range op.Parameters {
			byName[param.In+":"+param.Name] = param.Required
		}
		for _, param := range spec.params {
			location := "path"
			if param.in == paramQuery {
				location = "query"
			}
			if required, ok := byName[location+":"+param.name]; !ok {
				t.Errorf("%s: spec param %s:%s not documented", spec.operationID, location, param.name)
			} else if required != param.required {
				t.Errorf("%s: required flag differs for %s:%s", spec.operationID, location, param.name)
			}
		}
		checkParamConstraints(t, spec.operationID, op, spec.params)
	}

	// Every GET operation must be accounted for so coverage stays honest.
	var missing []string
	for _, op := range ops {
		if op.Method != "GET" || seen[op.OperationID] {
			continue
		}
		if _, exempt := readSpecExempt[op.OperationID]; exempt {
			continue
		}
		missing = append(missing, op.OperationID)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("GET operations without a read command or documented exemption: %v", missing)
	}

	// The binary classification must match a non-JSON 200 content type.
	for _, spec := range readSpecs {
		op := byID[spec.operationID]
		resp, ok := op.Responses["200"]
		if !ok {
			continue
		}
		nonJSON := false
		for contentType := range resp.Content {
			if !strings.Contains(contentType, "json") {
				nonJSON = true
			}
		}
		if nonJSON != spec.binary {
			t.Errorf("%s: binary = %v, 200 content types = %v", spec.operationID, spec.binary, resp.Content)
		}
	}
}

func TestEveryReadSpecIsRegistered(t *testing.T) {
	root := newTestEnv(t).app.newRootCommand()
	for _, spec := range readSpecs {
		group := findCommand(root, spec.group)
		if group == nil {
			t.Errorf("%s: group %q not registered", spec.operationID, spec.group)
			continue
		}
		if findCommand(group, spec.action) == nil {
			t.Errorf("%s: command %s %s not registered", spec.operationID, spec.group, spec.action)
		}
	}
}

func findCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func TestSecretReadRedaction(t *testing.T) {
	for _, command := range []struct {
		name  string
		args  []string
		field string
	}{
		{"oauth", []string{"oauth", "get-id-token"}, "idToken"},
		{"gitea", []string{"gitea", "get-integration", "--project-id", "p1"}, "webhookSecret"},
	} {
		t.Run(command.name, func(t *testing.T) {
			for _, test := range []struct {
				name string
				body string
				want string
			}{
				{"secret", `{"` + command.field + `":"synthetic-secret","number":9007199254740993}`, `{"` + command.field + `":"[REDACTED]","number":9007199254740993}`},
				{"null-field", `{"` + command.field + `":null}`, `{"` + command.field + `":null}`},
				{"absent", `{}`, `{}`},
				{"null-object", `null`, `null`},
				{"malformed", `{"` + command.field + `":"synthetic-secret"`, ""},
				{"trailing-data", `{"` + command.field + `":"synthetic-secret"} {}`, ""},
			} {
				t.Run(test.name, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(test.body))
					}))
					defer server.Close()
					env := newTestEnv(t)
					env.setAPIURL(server.URL + "/api")
					env.env["KANEO_TOKEN"] = "synthetic"
					status, stdout, stderr := env.run(command.args...)
					if strings.Contains(stdout+stderr, "synthetic-secret") {
						t.Fatal("secret leaked into output")
					}
					if test.want == "" {
						if status == 0 || stdout != "" {
							t.Fatalf("invalid response: status=%d stdout=%q", status, stdout)
						}
						return
					}
					var got, want map[string]json.RawMessage
					if err := json.Unmarshal([]byte(stdout), &got); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal([]byte(test.want), &want); err != nil {
						t.Fatal(err)
					}
					gotJSON, _ := json.Marshal(got)
					wantJSON, _ := json.Marshal(want)
					if status != 0 || string(gotJSON) != string(wantJSON) {
						t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
					}
				})
			}
		})
	}
}

func TestReadRequiredAndEnumFlags(t *testing.T) {
	env := newTestEnv(t)
	env.setAPIURL("https://example.com/api")

	if status, _, stderr := env.run("project", "get"); status != 2 || !strings.Contains(stderr, "--id is required") {
		t.Fatalf("missing required: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("task", "list", "--project-id", "p1", "--sort-order", "sideways"); status != 2 || !strings.Contains(stderr, "invalid_arguments") {
		t.Fatalf("enum: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("task", "list", "--project-id", "p1", "--page", "first"); status != 2 || !strings.Contains(stderr, "invalid_arguments") {
		t.Fatalf("numeric: status=%d stderr=%q", status, stderr)
	}
}

func TestReadPathEscapingAndQueryOmission(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	if status, _, stderr := env.run("task", "get", "--id", "a/b c"); status != 0 {
		t.Fatalf("task get: status=%d stderr=%q", status, stderr)
	}
	if !strings.HasSuffix(gotPath, "/api/task/a%2Fb%20c") {
		t.Fatalf("escaped path = %q", gotPath)
	}

	if status, _, stderr := env.run("project", "list", "--workspace-id", "ws 1"); status != 0 {
		t.Fatalf("project list: status=%d stderr=%q", status, stderr)
	}
	if gotQuery != "workspaceId=ws+1" {
		t.Fatalf("query = %q, want only the supplied workspaceId", gotQuery)
	}
}

func TestSearchGlobalHiddenQueryAlias(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	if status, _, stderr := env.run("search", "global", "--q", "foo", "--workspace-id", "W"); status != 0 {
		t.Fatalf("canonical: status=%d stderr=%q", status, stderr)
	}
	canonical := gotQuery
	if !strings.Contains(canonical, "q=foo") {
		t.Fatalf("canonical query = %q", canonical)
	}

	gotQuery = ""
	if status, _, stderr := env.run("search", "global", "--query", "foo", "--workspace-id", "W"); status != 0 {
		t.Fatalf("alias: status=%d stderr=%q", status, stderr)
	}
	if gotQuery != canonical {
		t.Fatalf("alias query = %q, want %q", gotQuery, canonical)
	}

	if status, stdout, stderr := env.run("search", "global", "--q", "a", "--query", "b", "--workspace-id", "W"); status == 0 {
		t.Fatalf("both flags accepted: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if status, _, stderr := env.run("search", "global", "--workspace-id", "W"); status != 2 || !strings.Contains(stderr, "--q is required") {
		t.Fatalf("neither flag: status=%d stderr=%q", status, stderr)
	}

	status, stdout, stderr := env.run("search", "global", "--help")
	if status != 0 {
		t.Fatalf("help: status=%d stderr=%q", status, stderr)
	}
	if strings.Contains(stdout+stderr, "--query") {
		t.Fatalf("help exposed the hidden alias: %q", stdout+stderr)
	}
	if !strings.Contains(stdout, "--q string") {
		t.Fatalf("help missing canonical flag: %q", stdout)
	}
}

func TestRoleSelectorRejectsAmbiguousOrEmptyInput(t *testing.T) {
	env := newTestEnv(t)
	env.setAPIURL("https://example.com/api")
	for _, args := range [][]string{
		{"org", "get-role"},
		{"org", "get-role", "--role-id", "id", "--role-name", "name"},
		{"org", "get-role", "--role-name", ""},
	} {
		if status, stdout, stderr := env.run(args...); status != 2 || stdout != "" {
			t.Fatalf("%v: status=%d stdout=%q stderr=%q", args, status, stdout, stderr)
		}
	}
}

// keyResolutionServer serves a project list and board pages for task key
// resolution and records the path (with query) of every request it receives.
// Board page N is served from pages[N-1]; a page past the end serves an empty
// board, as a server that runs out of tasks would.
func keyResolutionServer(t *testing.T, projects string, pages ...string) (*httptest.Server, *[]string) {
	t.Helper()
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		requests = append(requests, path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.EscapedPath(), "/api/project"):
			_, _ = w.Write([]byte(projects))
		case strings.Contains(r.URL.EscapedPath(), "/api/task/tasks/"):
			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			if err != nil || page < 1 {
				t.Errorf("board request without a valid page: %s", path)
				page = 1
			}
			if page > len(pages) {
				_, _ = w.Write([]byte(`{"data":{"columns":[]},"pagination":{"total":0,"page":` + strconv.Itoa(page) + `,"pageSize":100,"totalPages":1}}`))
				return
			}
			_, _ = w.Write([]byte(pages[page-1]))
		default:
			_, _ = w.Write([]byte(`{"id":"t-1","title":"resolved task"}`))
		}
	}))
	return server, &requests
}

const keyResolutionProjects = `[{"id":"p-1","slug":"KAN"},{"id":"p-2","slug":"OTHER"}]`

// boardPage is the query every board request carries: full pages sorted by
// number so the walk is stable and can stop once it has passed the target.
func boardPage(page int) string {
	return "/api/task/tasks/p-1?limit=100&page=" + strconv.Itoa(page) + "&sortBy=number&sortOrder=asc"
}

func TestTaskGetResolvesDisplayKey(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		board string
	}{
		{"column", "KAN-12", `{"data":{"columns":[{"tasks":[{"id":"t-9","number":11},{"id":"t-1","number":12}]}]}}`},
		{"case-insensitive", "kan-12", `{"data":{"columns":[{"tasks":[{"id":"t-1","number":12}]}]}}`},
		{"archived", "KAN-12", `{"data":{"columns":[],"archivedTasks":[{"id":"t-1","number":12}]}}`},
		{"planned", "KAN-12", `{"data":{"columns":[],"plannedTasks":[{"id":"t-1","number":12}]}}`},
		{"duplicate-across-buckets", "KAN-12", `{"data":{"columns":[{"tasks":[{"id":"t-1","number":12}]}],"archivedTasks":[{"id":"t-1","number":12}]}}`},
		{"null-number-skipped", "KAN-12", `{"data":{"columns":[{"tasks":[{"id":"t-null","number":null},{"id":"t-1","number":12}]}]}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, requests := keyResolutionServer(t, keyResolutionProjects, test.board)
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"

			status, stdout, stderr := env.run("task", "get", "--key", test.key, "--workspace-id", "W")
			if status != 0 {
				t.Fatalf("status=%d stderr=%q", status, stderr)
			}
			want := []string{"/api/project?includeArchived=true&workspaceId=W", boardPage(1), "/api/task/t-1"}
			if strings.Join(*requests, " ") != strings.Join(want, " ") {
				t.Fatalf("requests = %v, want %v", *requests, want)
			}
			if !strings.Contains(stdout, "resolved task") {
				t.Fatalf("stdout = %q", stdout)
			}
		})
	}
}

// pagedBoard renders board page `page` of `totalPages`, its column holding the
// given task numbers, with the pagination shape upstream sends.
func pagedBoard(page, totalPages int, numbers ...int) string {
	tasks := make([]string, 0, len(numbers))
	for _, number := range numbers {
		tasks = append(tasks, `{"id":"t-`+strconv.Itoa(number)+`","number":`+strconv.Itoa(number)+`}`)
	}
	return `{"data":{"columns":[{"tasks":[` + strings.Join(tasks, ",") + `]}]},"pagination":{"total":` + strconv.Itoa(totalPages*100) + `,"page":` + strconv.Itoa(page) + `,"pageSize":100,"totalPages":` + strconv.Itoa(totalPages) + `}}`
}

func TestTaskGetResolvesDisplayKeyAcrossPages(t *testing.T) {
	for _, test := range []struct {
		name         string
		key          string
		pages        []string
		wantStatus   int
		wantContains string
		wantBoards   int
	}{
		// The target sits on a later page: the walk continues past page 1.
		{"second-page", "KAN-5", []string{pagedBoard(1, 3, 1, 2, 3), pagedBoard(2, 3, 4, 5, 6), pagedBoard(3, 3, 7, 8, 9)}, 0, "resolved task", 2},
		// The target sits on the last page, reached only by trusting totalPages.
		{"last-page", "KAN-9", []string{pagedBoard(1, 3, 1, 2, 3), pagedBoard(2, 3, 4, 5, 6), pagedBoard(3, 3, 7, 8, 9)}, 0, "resolved task", 3},
		// A later bucket on the same page still counts, so a planned task on page 2 resolves.
		{"planned-on-second-page", "KAN-5", []string{pagedBoard(1, 2, 1, 2, 3), `{"data":{"columns":[],"plannedTasks":[{"id":"t-5","number":5}]},"pagination":{"total":4,"page":2,"pageSize":100,"totalPages":2}}`}, 0, "resolved task", 2},
		// A page whose tasks all sit outside every bucket (an unbucketed status) is not the end of the board.
		{"empty-middle-page", "KAN-9", []string{pagedBoard(1, 3, 1, 2), pagedBoard(2, 3), pagedBoard(3, 3, 9)}, 0, "resolved task", 3},
		// Ascending order has passed the target on page 1: stop without fetching the rest.
		{"stops-past-target", "KAN-2", []string{pagedBoard(1, 3, 1, 3), pagedBoard(2, 3, 4, 5), pagedBoard(3, 3, 6, 7)}, 2, "KAN-2", 1},
		// The page count comes from the first page; a later page cannot extend the walk.
		// Page 2 keeps page 1's total so only the page count differs.
		{"later-page-cannot-extend", "KAN-9", []string{pagedBoard(1, 2, 1, 2), `{"data":{"columns":[{"tasks":[{"id":"t-3","number":3},{"id":"t-4","number":4}]}]},"pagination":{"total":200,"page":2,"pageSize":100,"totalPages":5}}`, pagedBoard(3, 5, 9)}, 2, "KAN-9", 2},
		// A duplicate straddling a page boundary is still reported as ambiguous.
		{"ambiguous-across-pages", "KAN-2", []string{pagedBoard(1, 2, 1, 2), `{"data":{"columns":[{"tasks":[{"id":"t-2b","number":2},{"id":"t-3","number":3}]}]},"pagination":{"total":4,"page":2,"pageSize":100,"totalPages":2}}`}, 2, "matches 2 tasks", 2},
		// No pagination object at all is a single page.
		{"unpaginated-server", "KAN-9", []string{`{"data":{"columns":[{"tasks":[{"id":"t-1","number":1}]}]}}`, pagedBoard(2, 2, 9)}, 2, "KAN-9", 1},
		// A page count that is not a usable number is one page.
		{"unusable-page-count", "KAN-9", []string{`{"data":{"columns":[{"tasks":[{"id":"t-1","number":1}]}]},"pagination":{"totalPages":-3}}`, pagedBoard(2, 2, 9)}, 2, "KAN-9", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, requests := keyResolutionServer(t, keyResolutionProjects, test.pages...)
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"

			status, stdout, stderr := env.run("task", "get", "--key", test.key, "--workspace-id", "W")
			if status != test.wantStatus {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			want := []string{"/api/project?includeArchived=true&workspaceId=W"}
			for page := 1; page <= test.wantBoards; page++ {
				want = append(want, boardPage(page))
			}
			if test.wantStatus == 0 {
				want = append(want, "/api/task/t-"+strings.TrimPrefix(test.key, "KAN-"))
				if !strings.Contains(stdout, test.wantContains) {
					t.Fatalf("stdout = %q", stdout)
				}
			} else {
				if stdout != "" || !strings.Contains(stderr, test.wantContains) {
					t.Fatalf("stdout=%q stderr=%q, want mention of %q", stdout, stderr, test.wantContains)
				}
			}
			if strings.Join(*requests, " ") != strings.Join(want, " ") {
				t.Fatalf("requests = %v, want %v", *requests, want)
			}
		})
	}
}

func TestTaskGetKeyResolutionFailsOnLaterPageError(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		code int
	}{
		{"server-error", `{"error":"boom"}`, http.StatusInternalServerError},
		{"invalid-json", "not json", http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.EscapedPath()+"?"+r.URL.RawQuery)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.EscapedPath(), "/api/project"):
					_, _ = w.Write([]byte(keyResolutionProjects))
				case strings.Contains(r.URL.EscapedPath(), "/api/task/tasks/"):
					if r.URL.Query().Get("page") == "1" {
						_, _ = w.Write([]byte(pagedBoard(1, 3, 1, 2, 3)))
						return
					}
					w.WriteHeader(test.code)
					_, _ = w.Write([]byte(test.body))
				default:
					_, _ = w.Write([]byte(`{"id":"t-1","title":"resolved task"}`))
				}
			}))
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"

			// A failure partway through the walk is an error, never "no task".
			status, stdout, stderr := env.run("task", "get", "--key", "KAN-9", "--workspace-id", "W")
			if status == 0 || status == 2 || stdout != "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if strings.Contains(stderr, "no task") {
				t.Fatalf("stderr = %q, reported not-found for a failed page", stderr)
			}
			if len(requests) != 3 {
				t.Fatalf("requests = %v, want the project list and two pages", requests)
			}
		})
	}
}

func TestTaskGetKeyResolutionFailures(t *testing.T) {
	for _, test := range []struct {
		name         string
		args         []string
		board        string
		wantContains string
		wantRequests int
	}{
		{"unknown-slug", []string{"task", "get", "--key", "NOPE-1", "--workspace-id", "W"}, `{"data":{"columns":[]}}`, "NOPE", 1},
		{"unknown-number", []string{"task", "get", "--key", "KAN-99", "--workspace-id", "W"}, `{"data":{"columns":[{"tasks":[{"id":"t-1","number":12}]}]}}`, "KAN-99", 2},
		{"malformed-key", []string{"task", "get", "--key", "foo", "--workspace-id", "W"}, `{"data":{"columns":[]}}`, "KAN-12", 0},
		{"key-without-workspace", []string{"task", "get", "--key", "KAN-12"}, `{"data":{"columns":[]}}`, "", 0},
		{"id-and-key", []string{"task", "get", "--id", "t-1", "--key", "KAN-12", "--workspace-id", "W"}, `{"data":{"columns":[]}}`, "", 0},
		{"neither", []string{"task", "get"}, `{"data":{"columns":[]}}`, "", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, requests := keyResolutionServer(t, keyResolutionProjects, test.board)
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"

			status, stdout, stderr := env.run(test.args...)
			if status == 0 || stdout != "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if test.wantContains != "" && !strings.Contains(stderr, test.wantContains) {
				t.Fatalf("stderr = %q, want mention of %q", stderr, test.wantContains)
			}
			if len(*requests) != test.wantRequests {
				t.Fatalf("requests = %v, want %d", *requests, test.wantRequests)
			}
			for _, request := range *requests {
				if strings.HasPrefix(request, "/api/task/") && !strings.HasPrefix(request, "/api/task/tasks/") {
					t.Fatalf("issued a task fetch despite failed resolution: %v", *requests)
				}
			}
		})
	}
}

func TestTaskGetByIDSkipsResolution(t *testing.T) {
	server, requests := keyResolutionServer(t, keyResolutionProjects, `{"data":{"columns":[]}}`)
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	status, stdout, stderr := env.run("task", "get", "--id", "t-42")
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if len(*requests) != 1 || (*requests)[0] != "/api/task/t-42" {
		t.Fatalf("requests = %v", *requests)
	}
	if !strings.Contains(stdout, "resolved task") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestReadHelpRequiredMarkers(t *testing.T) {
	env := newTestEnv(t)
	env.setAPIURL("https://example.com/api")

	status, stdout, stderr := env.run("task", "get", "--help")
	if status != 0 {
		t.Fatalf("task get help: status=%d stderr=%q", status, stderr)
	}
	if !strings.Contains(stdout, "(required unless --key is given)") {
		t.Fatalf("task get help missing the conditional marker: %q", stdout)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "--id string") && strings.Contains(line, "(required)") {
			t.Fatalf("--id still claims to be unconditionally required: %q", line)
		}
	}

	// Specs without key resolution keep today's exact rendering.
	status, stdout, stderr = env.run("search", "global", "--help")
	if status != 0 {
		t.Fatalf("search global help: status=%d stderr=%q", status, stderr)
	}
	for _, want := range []string{"--q string", "--workspace-id string"} {
		found := false
		for _, line := range strings.Split(stdout, "\n") {
			if strings.Contains(line, want) && strings.HasSuffix(strings.TrimSpace(line), "(required)") {
				found = true
			}
		}
		if !found {
			t.Fatalf("search global help lost the plain (required) marker on %s: %q", want, stdout)
		}
	}
}

func TestAuthenticatedReadSendsBearer(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`null`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic-secret"

	status, stdout, stderr := env.run("org", "list")
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if auth != "Bearer synthetic-secret" {
		t.Fatalf("authorization = %q", auth)
	}
	if stdout != "null\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPublicReadOmitsCredentials(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("avatar"))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic-secret"

	if status, _, stderr := env.run("user", "download-avatar", "--id", "u1", "--output", "-"); status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if auth != "" {
		t.Fatalf("public read sent Authorization %q", auth)
	}
}

func TestBinaryDownloadRequiresOutputAndRefusesOverwrite(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G', 0x00, 0xff})
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")

	if status, _, stderr := env.run("user", "download-avatar", "--id", "u1"); status != 2 || !strings.Contains(stderr, "--output is required") {
		t.Fatalf("missing output: status=%d stderr=%q", status, stderr)
	}
	if requests != 0 {
		t.Fatal("missing output issued a request")
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "avatar.png")
	if status, _, stderr := env.run("user", "download-avatar", "--id", "u1", "--output", dest); status != 0 {
		t.Fatalf("download: status=%d stderr=%q", status, stderr)
	}
	data, err := os.ReadFile(dest)
	if err != nil || len(data) != 6 {
		t.Fatalf("downloaded %d bytes, err=%v", len(data), err)
	}
	if status, _, stderr := env.run("user", "download-avatar", "--id", "u1", "--output", dest); status != 2 || !strings.Contains(stderr, "--force") {
		t.Fatalf("overwrite: status=%d stderr=%q", status, stderr)
	}
	if requests != 1 {
		t.Fatal("overwrite refusal issued a request")
	}
	if status, _, stderr := env.run("user", "download-avatar", "--id", "u1", "--output", dest, "--force"); status != 0 {
		t.Fatalf("force overwrite: status=%d stderr=%q", status, stderr)
	}
}

func TestAssetDownloadSendsOptionalCredential(t *testing.T) {
	var withAuth, withoutAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			withAuth = r.Header.Get("Authorization")
		}
		withoutAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("bin"))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic-secret"
	if status, _, stderr := env.run("asset", "download", "--id", "a1", "--output", "-"); status != 0 {
		t.Fatalf("credentialed: status=%d stderr=%q", status, stderr)
	}
	if withAuth != "Bearer synthetic-secret" {
		t.Fatalf("asset download did not send the optional credential: %q", withAuth)
	}

	delete(env.env, "KANEO_TOKEN")
	if status, _, stderr := env.run("asset", "download", "--id", "a1", "--output", "-"); status != 0 {
		t.Fatalf("unauthenticated: status=%d stderr=%q", status, stderr)
	}
	if withoutAuth != "" {
		t.Fatalf("anonymous asset download sent %q", withoutAuth)
	}
}

func TestBinaryDownloadToStdout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("binary-body"))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	status, stdout, stderr := env.run("asset", "download", "--id", "a1", "--output", "-")
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if stdout != "binary-body" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestJSONResponseValidationAcrossChunks(t *testing.T) {
	for _, payload := range []string{`{"value":9007199254740993}`, `{"broken":`, `{} {}`, `<html>routing-secret</html>`} {
		t.Run(payload, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(" \n"))
				w.(http.Flusher).Flush()
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			status, stdout, stderr := env.run("org", "list")
			if json.Valid([]byte(payload)) {
				if status != 0 || stdout != " \n"+payload+"\n" {
					t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
				}
				return
			}
			if status != 1 || stdout != "" {
				t.Fatalf("invalid JSON: status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if !strings.Contains(stderr, "dashboard or proxy") || !strings.Contains(stderr, "/api") || !strings.Contains(stderr, "profile get") {
				t.Fatalf("stderr missing invalid-JSON guidance: %q", stderr)
			}
			if strings.Contains(stderr, "routing-secret") {
				t.Fatalf("stderr leaked response body: %q", stderr)
			}
		})
	}
}

func TestRedactedJSONResponseUsesSafeInvalidJSONGuidance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>redacted-routing-secret</html>"))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("gitea", "get-integration", "--project-id", "p1")
	if status != 1 || stdout != "" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if !strings.Contains(stderr, "dashboard or proxy") || strings.Contains(stderr, "redacted-routing-secret") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestTaskGetResolvesArchivedProject(t *testing.T) {
	// The project is only visible when archived projects are requested.
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		requests = append(requests, path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.EscapedPath(), "/api/project"):
			if r.URL.Query().Get("includeArchived") != "true" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":"p-1","slug":"KAN","isArchived":true}]`))
		case strings.Contains(r.URL.EscapedPath(), "/api/task/tasks/"):
			_, _ = w.Write([]byte(`{"data":{"columns":[],"archivedTasks":[{"id":"t-1","number":12}]}}`))
		default:
			_, _ = w.Write([]byte(`{"id":"t-1","title":"resolved task"}`))
		}
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	status, stdout, stderr := env.run("task", "get", "--key", "KAN-12", "--workspace-id", "W")
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	want := []string{"/api/project?includeArchived=true&workspaceId=W", boardPage(1), "/api/task/t-1"}
	if strings.Join(requests, " ") != strings.Join(want, " ") {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
	if !strings.Contains(stdout, "resolved task") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestTaskGetKeyResolutionAmbiguity(t *testing.T) {
	for _, test := range []struct {
		name         string
		projects     string
		board        string
		wantContains string
	}{
		{
			"ambiguous-slug",
			`[{"id":"p-1","slug":"KAN"},{"id":"p-2","slug":"kan"}]`,
			`{"data":{"columns":[{"tasks":[{"id":"t-1","number":12}]}]}}`,
			"matches 2 projects",
		},
		{
			"ambiguous-task",
			keyResolutionProjects,
			`{"data":{"columns":[{"tasks":[{"id":"t-1","number":12},{"id":"t-2","number":12}]}]}}`,
			"matches 2 tasks",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, requests := keyResolutionServer(t, test.projects, test.board)
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"

			status, stdout, stderr := env.run("task", "get", "--key", "KAN-12", "--workspace-id", "W")
			if status == 0 || stdout != "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if !strings.Contains(stderr, test.wantContains) {
				t.Fatalf("stderr = %q, want mention of %q", stderr, test.wantContains)
			}
			for _, request := range *requests {
				if strings.HasPrefix(request, "/api/task/") && !strings.HasPrefix(request, "/api/task/tasks/") {
					t.Fatalf("issued a task fetch despite ambiguous resolution: %v", *requests)
				}
			}
		})
	}
}

func TestTaskGetKeyResolutionRejectsOversizedBody(t *testing.T) {
	oversized := `[{"id":"p-1","slug":"KAN","pad":"` + strings.Repeat("a", maxRequestBody) + `"}]`
	oversizedBoard := `{"data":{"pad":"` + strings.Repeat("a", maxRequestBody) + `","columns":[{"tasks":[{"id":"t-1","number":12}]}]}}`
	for _, test := range []struct {
		name     string
		projects string
		board    string
	}{
		{"projects", oversized, `{"data":{"columns":[{"tasks":[{"id":"t-1","number":12}]}]}}`},
		{"board", keyResolutionProjects, oversizedBoard},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, _ := keyResolutionServer(t, test.projects, test.board)
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"

			status, stdout, stderr := env.run("task", "get", "--key", "KAN-12", "--workspace-id", "W")
			if status == 0 || stdout != "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if !strings.Contains(stderr, "exceeds") {
				t.Fatalf("stderr = %q, want a size limit failure", stderr)
			}
		})
	}
}

func TestTaskGetKeyResolutionRejectsNonJSONBody(t *testing.T) {
	for _, test := range []struct {
		name     string
		projects string
		board    string
	}{
		{"projects", "not json", `{"data":{"columns":[{"tasks":[{"id":"t-1","number":12}]}]}}`},
		{"board", keyResolutionProjects, "not json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, requests := keyResolutionServer(t, test.projects, test.board)
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"

			status, stdout, stderr := env.run("task", "get", "--key", "KAN-12", "--workspace-id", "W")
			if status == 0 || stdout != "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			for _, request := range *requests {
				if strings.HasPrefix(request, "/api/task/") && !strings.HasPrefix(request, "/api/task/tasks/") {
					t.Fatalf("issued a task fetch despite an unusable body: %v", *requests)
				}
			}
		})
	}
}

func TestTaskGetKeyResolutionRejectsEmptyWorkspaceID(t *testing.T) {
	server, requests := keyResolutionServer(t, keyResolutionProjects, `{"data":{"columns":[]}}`)
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	status, stdout, stderr := env.run("task", "get", "--key", "KAN-12", "--workspace-id", "")
	if status != 2 || stdout != "" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if !strings.Contains(stderr, "--workspace-id is required") {
		t.Fatalf("stderr = %q", stderr)
	}
	if len(*requests) != 0 {
		t.Fatalf("requests = %v, want none", *requests)
	}
}

func TestNavigationHelpMarksRequiredFlags(t *testing.T) {
	env := newTestEnv(t)
	status, stdout, stderr := env.run("mcp", "start-authorization", "--help")
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	for _, want := range []string{"OAuth client ID (required)", "PKCE code challenge method (one of: S256) (required)"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "OAuth state (required)") {
		t.Fatalf("optional flag marked required: %q", stdout)
	}
}
