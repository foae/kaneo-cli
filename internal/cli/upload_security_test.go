package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTaskImageUploadEscapesTaskID(t *testing.T) {
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer storage.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.EscapedPath(), "/api/task/image-upload/task%2Fone%3Fdraft=true"; got != want {
			t.Errorf("escaped path = %q, want %q", got, want)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("raw query = %q, want empty", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":       "tasks/image.png",
			"uploadUrl": storage.URL + "/upload",
		})
	}))
	defer api.Close()

	imagePath := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	env := newTestEnv(t)
	env.setAPIURL(api.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	status, stdout, stderr := env.run("task", "create-image-upload", "--id", "task/one?draft=true", "--file", imagePath, "--surface", "description")
	if status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
	if stdout != "{\"key\":\"tasks/image.png\"}\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestTaskImageUploadRejectsInvalidTaskIDsBeforeNetwork(t *testing.T) {
	for _, taskID := range []string{"", ".", ".."} {
		t.Run("id="+taskID, func(t *testing.T) {
			var requests atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer api.Close()

			imagePath := filepath.Join(t.TempDir(), "image.png")
			if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
				t.Fatalf("write image: %v", err)
			}
			env := newTestEnv(t)
			env.setAPIURL(api.URL + "/api")

			status, _, stderr := env.run("task", "create-image-upload", "--id", taskID, "--file", imagePath, "--surface", "description")
			if status != 2 || commandErrorCode(t, stderr) != "invalid_arguments" {
				t.Fatalf("status = %d, stderr = %q", status, stderr)
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("API requests = %d, want 0", got)
			}
		})
	}
}

func TestTaskImageUploadRefusesStorageRedirect(t *testing.T) {
	var redirectedRequests atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectedRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer redirectTarget.Close()

	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Storage-Signature"); got != "signed-value" {
			t.Errorf("initial storage signature = %q", got)
		}
		http.Redirect(w, r, redirectTarget.URL+"/redirected?signature=location-secret", http.StatusFound)
	}))
	defer storage.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":       "tasks/image.png",
			"uploadUrl": storage.URL + "/upload?signature=initial-secret",
			"headers":   map[string]string{"X-Storage-Signature": "signed-value"},
		})
	}))
	defer api.Close()

	imagePath := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	env := newTestEnv(t)
	env.setAPIURL(api.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	status, stdout, stderr := env.run("task", "create-image-upload", "--id", "task", "--file", imagePath, "--surface", "description")
	if status != 1 || commandErrorCode(t, stderr) != "process_failure" {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if got := redirectedRequests.Load(); got != 0 {
		t.Fatalf("redirect target requests = %d, want 0", got)
	}
	if strings.Contains(stderr, "initial-secret") || strings.Contains(stderr, "location-secret") {
		t.Fatalf("stderr exposed presigned URL secret: %q", stderr)
	}
}

func TestTaskImageUploadRejectsUnsafePresignResponses(t *testing.T) {
	tests := []struct {
		name     string
		response func(http.ResponseWriter, string)
	}{
		{
			name: "missing key",
			response: func(w http.ResponseWriter, storageURL string) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"uploadUrl": storageURL + "/upload?signature=presign-secret",
				})
			},
		},
		{
			name: "oversized body",
			response: func(w http.ResponseWriter, _ string) {
				_, _ = w.Write(make([]byte, maxRequestBody+1))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var storageRequests atomic.Int32
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				storageRequests.Add(1)
				w.WriteHeader(http.StatusNoContent)
			}))
			defer storage.Close()

			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				tt.response(w, storage.URL)
			}))
			defer api.Close()

			imagePath := filepath.Join(t.TempDir(), "image.png")
			if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
				t.Fatalf("write image: %v", err)
			}
			env := newTestEnv(t)
			env.setAPIURL(api.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"

			status, stdout, stderr := env.run("task", "create-image-upload", "--id", "task", "--file", imagePath, "--surface", "description")
			if status != 1 || commandErrorCode(t, stderr) != "process_failure" {
				t.Fatalf("status = %d, stderr = %q", status, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if got := storageRequests.Load(); got != 0 {
				t.Fatalf("storage requests = %d, want 0", got)
			}
			if strings.Contains(stderr, "presign-secret") {
				t.Fatalf("stderr exposed presigned URL secret: %q", stderr)
			}
		})
	}
}

func TestReadBodyFileRejectsOversizedAndNonregularInputs(t *testing.T) {
	oversized := filepath.Join(t.TempDir(), "oversized")
	if err := os.WriteFile(oversized, make([]byte, maxRequestBody+1), 0o600); err != nil {
		t.Fatalf("write oversized file: %v", err)
	}
	if _, err := readBodyFile(oversized); !isUsageFileError(err) {
		t.Fatalf("oversized file error = %v, want usage error", err)
	}

	if _, err := readBodyFile(t.TempDir()); !isUsageFileError(err) {
		t.Fatalf("directory error = %v, want usage error", err)
	}
}

func TestTaskImageUploadStorageTimeoutUsesTimeoutCode(t *testing.T) {
	release := make(chan struct{})
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer storage.Close()
	defer close(release)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":       "tasks/image.png",
			"uploadUrl": storage.URL + "/upload",
		})
	}))
	defer api.Close()

	imagePath := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	env := newTestEnv(t)
	env.setAPIURL(api.URL + "/api")
	env.env["KANEO_TOKEN"] = "synthetic"

	status, _, stderr := env.run("--timeout", "25ms", "task", "create-image-upload", "--id", "task", "--file", imagePath, "--surface", "description")
	if status != 4 || commandErrorCode(t, stderr) != "timeout" {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
}

func commandErrorCode(t *testing.T, stderr string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	var response errorResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatalf("stderr is not JSON: %v (%q)", err, stderr)
	}
	return response.Error.Code
}

func isUsageFileError(err error) bool {
	var usage *usageError
	return errors.As(err, &usage)
}
