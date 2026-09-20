package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/foae/kaneo-cli/internal/buildinfo"
)

func TestExecuteRoutesInputErrorsToJSONStderr(t *testing.T) {
	info := buildinfo.Info{Version: "v0.1.0", Commit: "abc123", Date: "2026-09-20T00:00:00Z"}

	tests := []struct {
		name string
		args []string
		code string
	}{
		{name: "unknown command", args: []string{"missing"}, code: "unknown_command"},
		{name: "unexpected argument", args: []string{"version", "extra"}, code: "invalid_arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			status := Execute(tt.args, bytes.NewReader(nil), &stdout, &stderr, info)
			if status != 2 {
				t.Fatalf("Execute() status = %d, want 2", status)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Execute() wrote %q to stdout, want no stdout", stdout.String())
			}

			var response errorResponse
			if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
				t.Fatalf("stderr is not JSON: %v", err)
			}
			if response.Error.Code != tt.code {
				t.Fatalf("error code = %q, want %q", response.Error.Code, tt.code)
			}
			if response.Error.Message == "" {
				t.Fatal("error message is empty")
			}
		})
	}
}
