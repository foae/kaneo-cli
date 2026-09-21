package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sizedJSONResponse(size int) []byte {
	const prefix = `{"value":"synthetic-response-secret`
	const suffix = `"}`
	return []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
}

func TestJSONResponseSizeLimits(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		batch      bool
		size       int
		wantStatus int
	}{
		{
			name:       "ordinary accepts limit",
			args:       []string{"org", "list"},
			size:       maxRequestBody,
			wantStatus: 0,
		},
		{
			name:       "ordinary rejects limit plus one",
			args:       []string{"org", "list"},
			size:       maxRequestBody + 1,
			wantStatus: 1,
		},
		{
			name:       "batch accepts limit",
			args:       []string{"task", "bulk-update", "--yes", "--body-file", "-"},
			batch:      true,
			size:       maxRequestBody,
			wantStatus: 0,
		},
		{
			name:       "batch rejects limit plus one",
			args:       []string{"task", "bulk-update", "--yes", "--body-file", "-"},
			batch:      true,
			size:       maxRequestBody + 1,
			wantStatus: 1,
		},
		{
			name:       "redacted accepts limit",
			args:       []string{"auth", "get-session"},
			size:       maxRequestBody,
			wantStatus: 0,
		},
		{
			name:       "redacted rejects limit plus one",
			args:       []string{"auth", "get-session"},
			size:       maxRequestBody + 1,
			wantStatus: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := sizedJSONResponse(tt.size)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(payload)
			}))
			defer server.Close()

			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			if tt.batch {
				env.setStdin(`{}`)
			}
			status, stdout, stderr := env.run(tt.args...)
			if status != tt.wantStatus {
				t.Fatalf("status=%d stdout_len=%d stderr=%q", status, len(stdout), stderr)
			}
			if tt.wantStatus == 0 {
				if stdout != string(payload)+"\n" {
					t.Fatalf("stdout did not preserve payload: length=%d want=%d", len(stdout), len(payload)+1)
				}
				return
			}
			if stdout != "" {
				t.Fatalf("oversized response wrote partial stdout: length=%d", len(stdout))
			}
			if !strings.Contains(stderr, "response exceeds size limit") || strings.Contains(stderr, "synthetic-response-secret") {
				t.Fatalf("stderr = %q", stderr)
			}
		})
	}
}
