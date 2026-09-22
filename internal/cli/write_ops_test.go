package cli

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fullyCoveredOperations names the operations outside the generic write table
// that are nonetheless implemented, so the coverage
// check can account for every pinned operation.
var fullyCoveredOperations = map[string]string{
	"getSession":                 "packet 1",
	"getConfig":                  "packet 1",
	"getInstanceStatus":          "packet 1",
	"getDeviceAuthorizationPage": "dedicated browser URL handoff",
	"authorizeMcpOAuthClient":    "dedicated browser URL handoff",
	"createTaskImageUpload":      "dedicated task create-image-upload",
	"uploadUserAvatar":           "dedicated user upload-avatar",
}

func TestWriteSpecsMatchInventory(t *testing.T) {
	ops := inventoryOperations(t)
	byID := make(map[string]inventoryOperation, len(ops))
	for _, op := range ops {
		byID[op.OperationID] = op
	}
	for _, spec := range writeSpecs {
		op, ok := byID[spec.operationID]
		if !ok {
			t.Errorf("%s: operation not in inventory", spec.operationID)
			continue
		}
		if op.Method != spec.method {
			t.Errorf("%s: method = %s, want %s", spec.operationID, op.Method, spec.method)
		}
		if op.Path != spec.path {
			t.Errorf("%s: path = %s, want %s", spec.operationID, op.Path, spec.path)
		}
		if op.Command.Group != spec.group || op.Command.Action != spec.action {
			t.Errorf("%s: command mismatch", spec.operationID)
		}
		if wantBody := op.RequestBody != nil; wantBody != spec.body {
			t.Errorf("%s: body = %v, inventory has body = %v", spec.operationID, spec.body, wantBody)
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
		if spec.method == "DELETE" && !spec.destructive {
			t.Errorf("%s: DELETE method must be destructive", spec.operationID)
		}
	}
}

func TestEveryOperationIsCovered(t *testing.T) {
	ops := inventoryOperations(t)
	specs := make(map[string]bool)
	for _, spec := range readSpecs {
		specs[spec.operationID] = true
	}
	for _, spec := range writeSpecs {
		specs[spec.operationID] = true
	}
	var missing []string
	for _, op := range ops {
		if specs[op.OperationID] {
			continue
		}
		if _, ok := fullyCoveredOperations[op.OperationID]; ok {
			continue
		}
		missing = append(missing, op.OperationID)
	}
	if len(missing) > 0 {
		t.Errorf("operations with no command and no documented exemption: %v", missing)
	}
}

func writeBodyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write body: %v", err)
	}
	return path
}

func TestWriteRejectsInvalidBody(t *testing.T) {
	env := newTestEnv(t)
	env.setAPIURL("https://example.com/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	missing := filepath.Join(t.TempDir(), "missing.json")
	if status, _, stderr := env.run("project", "create", "--body-file", missing); status != 1 || !strings.Contains(stderr, "process_failure") {
		t.Fatalf("missing file: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("project", "create", "--body-file", writeBodyFile(t, "not json")); status != 2 || !strings.Contains(stderr, "invalid_arguments") {
		t.Fatalf("invalid json: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("project", "create", "--body-file", writeBodyFile(t, "[]")); status != 2 || !strings.Contains(stderr, "JSON object") {
		t.Fatalf("non-object: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("project", "create"); status != 2 || !strings.Contains(stderr, "--body-file is required") {
		t.Fatalf("missing flag: status=%d stderr=%q", status, stderr)
	}
}

func TestDestructiveRequiresYesBeforeNetwork(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	if status, _, stderr := env.run("task", "delete", "--id", "t1"); status != 2 || !strings.Contains(stderr, "without --yes") {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if called {
		t.Fatal("destructive request reached the server without --yes")
	}
	if status, _, stderr := env.run("org", "delete"); status != 2 || !strings.Contains(stderr, "without --yes") {
		t.Fatalf("org delete: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("notification", "clear-all"); status != 2 || !strings.Contains(stderr, "without --yes") {
		t.Fatalf("clear-all: status=%d stderr=%q", status, stderr)
	}
}

func TestWritePassesBodyThroughUnchanged(t *testing.T) {
	var gotBody []byte
	var gotMethod, gotPath, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	}))
	defer server.Close()

	original := `{"name":"Big","estimate":12345678901234567890,"unknown":{"k":[1,2,3]}}`
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("project", "create", "--body-file", writeBodyFile(t, original))
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if gotMethod != "POST" || gotPath != "/api/project" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if string(gotBody) != original {
		t.Fatalf("body altered: %s", gotBody)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content type = %q", gotContentType)
	}
	if stdout != "{\"id\":\"p1\"}\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPublicWriteOmitsCredentials(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"client_id":"c1"}`))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, _, stderr := env.run("mcp", "register-oauth-client", "--body-file", writeBodyFile(t, `{"client_name":"x"}`))
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if auth != "" {
		t.Fatalf("public write sent Authorization %q", auth)
	}
}

func TestTaskImageUploadStreamsToStorageWithoutCredential(t *testing.T) {
	var storageAuth string
	var storageBody []byte
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageAuth = r.Header.Get("Authorization")
		storageBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer storage.Close()

	var apiBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&apiBody)
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{
			"key":       "tasks/t1/img.png",
			"uploadUrl": storage.URL + "/upload?signature=abc",
			"headers":   map[string]string{"Content-Type": "image/png"},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer api.Close()

	imagePath := filepath.Join(t.TempDir(), "image.png")
	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	if err := os.WriteFile(imagePath, imageBytes, 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}

	env := newTestEnv(t)
	env.setAPIURL(api.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	status, stdout, stderr := env.run("task", "create-image-upload",
		"--id", "t1", "--file", imagePath, "--surface", "description")
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if apiBody["filename"] != "image.png" || apiBody["contentType"] != "image/png" || apiBody["surface"] != "description" {
		t.Fatalf("api body = %v", apiBody)
	}
	if apiBody["size"].(float64) != float64(len(imageBytes)) {
		t.Fatalf("size = %v", apiBody["size"])
	}
	if storageAuth != "" {
		t.Fatalf("storage upload carried Authorization %q", storageAuth)
	}
	if string(storageBody) != string(imageBytes) {
		t.Fatalf("storage body = %v", storageBody)
	}
	if strings.Contains(stdout+stderr, "signature=abc") || strings.Contains(stdout, "uploadUrl") || strings.Contains(stdout, "headers") {
		t.Fatal("upload output exposed presigned transfer credentials")
	}
	if !strings.Contains(stdout, "tasks/t1/img.png") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestUploadAvatarEncodesFile(t *testing.T) {
	var got map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"a1","url":"/api/user/avatar/a1","size":3}`))
	}))
	defer server.Close()

	avatarPath := filepath.Join(t.TempDir(), "me.png")
	want := []byte{1, 2, 3}
	if err := os.WriteFile(avatarPath, want, 0o600); err != nil {
		t.Fatalf("write avatar: %v", err)
	}

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"
	if status, _, stderr := env.run("user", "upload-avatar", "--file", avatarPath); status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if got["contentType"] != "image/png" {
		t.Fatalf("content type = %q", got["contentType"])
	}
	decoded, err := base64.StdEncoding.DecodeString(got["data"])
	if err != nil || string(decoded) != string(want) {
		t.Fatalf("decoded = %v err=%v", decoded, err)
	}

	big := filepath.Join(t.TempDir(), "big.png")
	if err := os.WriteFile(big, make([]byte, maxAvatarBytes+1), 0o600); err != nil {
		t.Fatalf("write big: %v", err)
	}
	if status, _, stderr := env.run("user", "upload-avatar", "--file", big); status != 2 || !strings.Contains(stderr, "exceeds") {
		t.Fatalf("oversize: status=%d stderr=%q", status, stderr)
	}
	if status, _, stderr := env.run("user", "upload-avatar", "--file", avatarPath, "--content-type", "text/plain"); status != 2 {
		t.Fatalf("bad content type: status=%d stderr=%q", status, stderr)
	}
}

func TestBatchPartialFailurePreservesResult(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		payload string
	}{
		{"bulk", []string{"task", "bulk-update", "--yes"}, `{"success":false,"updatedCount":1}`},
		{"tasks", []string{"task", "import", "--project-id", "p1"}, `{"results":{"total":2,"successful":1,"failed":1,"tasks":[]}}`},
		{"github", []string{"github", "import-issues"}, `{"imported":1,"skipped":1,"errors":["synthetic failure"]}`},
		{"gitea", []string{"gitea", "import-issues"}, `{"imported":1,"updated":0,"skipped":1,"errors":["synthetic failure"]}`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tt.payload)
			}))
			defer server.Close()
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			args := append(tt.args, "--body-file", writeBodyFile(t, `{}`))
			status, stdout, stderr := env.run(args...)
			if status != 5 || stdout != tt.payload+"\n" || !strings.Contains(stderr, `"code":"partial_failure"`) {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
		})
	}
}

func TestBatchInvalidJSONUsesSafeGuidance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>batch-routing-secret</html>"))
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	status, stdout, stderr := env.run("task", "bulk-update", "--yes", "--body-file", writeBodyFile(t, `{}`))
	if status != 1 || stdout != "" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if !strings.Contains(stderr, "dashboard or proxy") || strings.Contains(stderr, "batch-routing-secret") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestMutationBodyCannotRedirectWithoutBearer(t *testing.T) {
	requests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = io.WriteString(w, `{}`)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	status, stdout, stderr := env.run("gitea", "create-integration", "--project-id", "p1", "--body-file", writeBodyFile(t, `{"token":"synthetic-secret"}`))
	if status != 4 || requests != 0 || stdout != "" || !strings.Contains(stderr, "cross_origin_redirect_refused") {
		t.Fatalf("status=%d requests=%d stdout=%q stderr=%q", status, requests, stdout, stderr)
	}
}

func TestWriteHelpMarksRequiredFlags(t *testing.T) {
	env := newTestEnv(t)
	status, stdout, stderr := env.run("task", "create", "--help")
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	if !strings.Contains(stdout, "--project-id string") || !strings.Contains(stdout, "(required)") {
		t.Fatalf("stdout = %q", stdout)
	}
}
