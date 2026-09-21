package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
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

type inventoryOperation struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	OperationID string `json:"operation_id"`
	Command     struct {
		Group  string `json:"group"`
		Action string `json:"action"`
	} `json:"command"`
	Parameters []struct {
		In       string `json:"in"`
		Name     string `json:"name"`
		Required bool   `json:"required"`
	} `json:"parameters"`
	RequestBody *json.RawMessage `json:"request_body"`
	Security    []map[string]any `json:"security"`
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
	for _, payload := range []string{`{"value":9007199254740993}`, `{"broken":`, `{} {}`} {
		t.Run(payload, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
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
			} else if status == 0 || stdout != "" {
				t.Fatalf("invalid JSON: status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
		})
	}
}
