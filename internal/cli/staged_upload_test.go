package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTaskUploadRequestContracts(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		idFlag      string
		id          string
		method      string
		path        string
		fileName    string
		filename    string
		contentType string
		surface     string
		extra       []string
	}{
		{
			name: "staged non-image attachment", action: "stage-asset-upload", idFlag: "project-id", id: "p1",
			method: "POST", path: "/api/task/draft-upload/p1", fileName: "local.bin",
			filename: "report.pdf", contentType: "application/pdf", surface: "description",
			extra: []string{"--filename", "report.pdf", "--content-type", "application/pdf"},
		},
		{
			name: "staged inferred image", action: "stage-asset-upload", idFlag: "project-id", id: "p1",
			method: "POST", path: "/api/task/draft-upload/p1", fileName: "image.PNG",
			filename: "image.PNG", contentType: "image/png", surface: "description",
		},
		{
			name: "staged escaped project", action: "stage-asset-upload", idFlag: "project-id", id: "project/one?draft=true",
			method: "POST", path: "/api/task/draft-upload/project%2Fone%3Fdraft=true", fileName: "image.png",
			filename: "image.png", contentType: "image/png", surface: "description",
		},
		{
			name: "existing task comment image", action: "create-image-upload", idFlag: "id", id: "t1",
			method: "PUT", path: "/api/task/image-upload/t1", fileName: "image.png",
			filename: "image.png", contentType: "image/png", surface: "comment",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileBytes := []byte{0, 1, 2, 3, 0xff, '\r', '\n', 0}
			filePath := filepath.Join(t.TempDir(), tt.fileName)
			if err := os.WriteFile(filePath, fileBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			var apiRequests, storageRequests atomic.Int32
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				storageRequests.Add(1)
				if r.Method != http.MethodPut || r.URL.Path != "/upload" {
					t.Errorf("storage request = %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("storage received API credentials")
				}
				if r.Header.Get("Content-Type") != tt.contentType || r.Header.Get("X-Storage-Signature") != "storage-header-secret" {
					t.Errorf("storage transfer headers do not match issuance")
				}
				if r.ContentLength != int64(len(fileBytes)) {
					t.Errorf("storage content length = %d", r.ContentLength)
				}
				got, err := io.ReadAll(r.Body)
				if err != nil || !bytes.Equal(got, fileBytes) {
					t.Errorf("storage bytes = %v, error = %v", got, err)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer storage.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				apiRequests.Add(1)
				if r.Method != tt.method || r.URL.EscapedPath() != tt.path || r.URL.RawQuery != "" {
					t.Errorf("API request = %s %s", r.Method, r.URL.RequestURI())
				}
				if r.Header.Get("Authorization") != "Bearer synthetic-api-token" {
					t.Error("API request did not carry configured bearer credential")
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("API content type = %q", r.Header.Get("Content-Type"))
				}
				var got map[string]any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if len(got) != 4 || got["filename"] != tt.filename || got["contentType"] != tt.contentType || got["size"] != float64(len(fileBytes)) || got["surface"] != tt.surface {
					t.Errorf("API body = %v", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"key": "uploads/safe-key", "uploadUrl": storage.URL + "/upload?signature=storage-url-secret",
					"headers": map[string]string{"Content-Type": tt.contentType, "X-Storage-Signature": "storage-header-secret"},
				})
			}))
			defer api.Close()
			env := newTestEnv(t)
			env.setAPIURL(api.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic-api-token"
			args := []string{"task", tt.action, "--" + tt.idFlag, tt.id, "--file", filePath, "--surface", tt.surface}
			args = append(args, tt.extra...)
			status, stdout, stderr := env.run(args...)
			if status != 0 || stdout != "{\"key\":\"uploads/safe-key\"}\n" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			for _, secret := range []string{"synthetic-api-token", "storage-url-secret", "storage-header-secret"} {
				if strings.Contains(stdout+stderr, secret) {
					t.Fatal("upload output disclosed credentials or storage authorization")
				}
			}
			if apiRequests.Load() != 1 || storageRequests.Load() != 1 {
				t.Fatalf("API requests=%d storage requests=%d", apiRequests.Load(), storageRequests.Load())
			}
		})
	}
}

func TestStagedUploadRejectsInvalidInputsBeforeNetwork(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(filePath, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(unknown, []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		project string
		file    string
		surface string
		status  int
	}{
		{"missing project", "", filePath, "description", 2},
		{"dot project", ".", filePath, "description", 2},
		{"parent project", "..", filePath, "description", 2},
		{"missing file", "p1", "", "description", 2},
		{"missing surface", "p1", filePath, "", 2},
		{"comment surface", "p1", filePath, "comment", 2},
		{"unknown surface", "p1", filePath, "other", 2},
		{"unknown extension", "p1", unknown, "description", 2},
		{"directory", "p1", t.TempDir(), "description", 2},
		{"missing local file", "p1", filepath.Join(t.TempDir(), "missing.png"), "description", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer api.Close()
			env := newTestEnv(t)
			env.setAPIURL(api.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic-api-token"
			status, stdout, stderr := env.run("task", "stage-asset-upload", "--project-id", tt.project, "--file", tt.file, "--surface", tt.surface)
			wantCode := "invalid_arguments"
			if tt.status == 1 {
				wantCode = "process_failure"
			}
			if status != tt.status || stdout != "" || commandErrorCode(t, stderr) != wantCode {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if requests.Load() != 0 {
				t.Fatalf("API requests=%d, want zero", requests.Load())
			}
		})
	}
}

func TestTaskUploadsRejectFIFOFilesBeforeNetwork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX FIFOs are not available to native Windows processes")
	}
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo is unavailable on this platform")
	}
	filePath := filepath.Join(t.TempDir(), "pipe.png")
	if err := exec.Command(mkfifo, filePath).Run(); err != nil {
		t.Fatalf("create FIFO: %v", err)
	}
	for _, command := range []struct{ action, flag string }{
		{"stage-asset-upload", "project-id"},
		{"create-image-upload", "id"},
	} {
		t.Run(command.action, func(t *testing.T) {
			var requests atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer api.Close()
			env := newTestEnv(t)
			env.setAPIURL(api.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic-api-token"
			status, stdout, stderr := env.run("task", command.action, "--"+command.flag, "one", "--file", filePath, "--surface", "description")
			if status != 2 || stdout != "" || commandErrorCode(t, stderr) != "invalid_arguments" || !strings.Contains(stderr, "not a regular file") {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if requests.Load() != 0 {
				t.Fatalf("API requests=%d, want zero", requests.Load())
			}
		})
	}
}

func TestStagedUploadFailuresNeverEmitKey(t *testing.T) {
	tests := []struct {
		name          string
		apiStatus     int
		response      string
		storageStatus int
		storageCalls  int32
	}{
		{"issuance unavailable", http.StatusServiceUnavailable, `{"message":"storage unavailable"}`, http.StatusNoContent, 0},
		{"issuance unauthorized", http.StatusUnauthorized, `{"message":"unauthorized"}`, http.StatusNoContent, 0},
		{"invalid issuance JSON", http.StatusOK, `<html>issuance-secret</html>`, http.StatusNoContent, 0},
		{"missing key", http.StatusOK, `{"uploadUrl":"STORAGE/upload?signature=url-secret","headers":{}}`, http.StatusNoContent, 0},
		{"missing URL", http.StatusOK, `{"key":"uploads/safe-key","headers":{}}`, http.StatusNoContent, 0},
		{"oversized issuance", http.StatusOK, strings.Repeat("x", maxRequestBody+1), http.StatusNoContent, 0},
		{"unsafe URL", http.StatusOK, `{"key":"uploads/safe-key","uploadUrl":"https://user:password-secret@example.com/upload"}`, http.StatusNoContent, 0},
		{"storage failure", http.StatusOK, `{"key":"uploads/safe-key","uploadUrl":"STORAGE/upload?signature=url-secret","headers":{"X-Storage-Signature":"header-secret"}}`, http.StatusForbidden, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var apiRequests, storageRequests atomic.Int32
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				storageRequests.Add(1)
				w.WriteHeader(tt.storageStatus)
				_, _ = w.Write([]byte("storage-response-secret"))
			}))
			defer storage.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				apiRequests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.apiStatus)
				_, _ = w.Write([]byte(strings.ReplaceAll(tt.response, "STORAGE", storage.URL)))
			}))
			defer api.Close()
			filePath := filepath.Join(t.TempDir(), "image.png")
			if err := os.WriteFile(filePath, []byte("image"), 0o600); err != nil {
				t.Fatal(err)
			}
			env := newTestEnv(t)
			env.setAPIURL(api.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic-api-token"
			status, stdout, stderr := env.run("task", "stage-asset-upload", "--project-id", "p1", "--file", filePath, "--surface", "description")
			if status == 0 || stdout != "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if apiRequests.Load() != 1 || storageRequests.Load() != tt.storageCalls {
				t.Fatalf("API requests=%d storage requests=%d, want 1 and %d", apiRequests.Load(), storageRequests.Load(), tt.storageCalls)
			}
			for _, secret := range []string{"issuance-secret", "url-secret", "header-secret", "password-secret", "storage-response-secret", "synthetic-api-token", "uploads/safe-key"} {
				if strings.Contains(stdout+stderr, secret) {
					t.Fatalf("failure output exposed %q", secret)
				}
			}
		})
	}
}

func TestStagedUploadRefusesStorageRedirect(t *testing.T) {
	var redirectedRequests, storageRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectedRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageRequests.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Storage-Signature") != "header-secret" {
			t.Error("unexpected storage credentials")
		}
		http.Redirect(w, r, target.URL+"/redirect?signature=redirect-secret", http.StatusTemporaryRedirect)
	}))
	defer storage.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key": "uploads/safe-key", "uploadUrl": storage.URL + "/upload?signature=url-secret",
			"headers": map[string]string{"X-Storage-Signature": "header-secret"},
		})
	}))
	defer api.Close()
	filePath := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(filePath, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := newTestEnv(t)
	env.setAPIURL(api.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic-api-token"
	status, stdout, stderr := env.run("task", "stage-asset-upload", "--project-id", "p1", "--file", filePath, "--surface", "description")
	if status != 1 || stdout != "" || commandErrorCode(t, stderr) != "process_failure" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if storageRequests.Load() != 1 || redirectedRequests.Load() != 0 {
		t.Fatalf("storage requests=%d redirected requests=%d", storageRequests.Load(), redirectedRequests.Load())
	}
	for _, secret := range []string{"url-secret", "header-secret", "redirect-secret", "uploads/safe-key"} {
		if strings.Contains(stdout+stderr, secret) {
			t.Fatalf("redirect failure output exposed %q", secret)
		}
	}
}
