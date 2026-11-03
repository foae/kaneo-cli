package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestUsageRecoveryRejectsInputWithoutRequests(t *testing.T) {
	const suppliedValue = "private-flag-value"
	for _, tt := range []struct {
		name string
		args []string
		code string
		want []string
	}{
		{
			name: "unknown root command",
			args: []string{"tas"},
			code: "unknown_command",
			want: []string{"kaneo-cli task", "kaneo-cli --help"},
		},
		{
			name: "unknown root without suggestion",
			args: []string{"not-a-real-command"},
			code: "unknown_command",
			want: []string{"kaneo-cli --help"},
		},
		{
			name: "comment create is not an alias",
			args: []string{"comment", "create"},
			code: "invalid_arguments",
			want: []string{"kaneo-cli comment create-task", "kaneo-cli comment --help"},
		},
		{
			name: "comment create with flags is not an alias",
			args: []string{"comment", "create", "--task-id", suppliedValue, "--body-file", "-"},
			code: "invalid_arguments",
			want: []string{"kaneo-cli comment create-task", "kaneo-cli comment --help"},
		},
		{
			name: "comment list is not an alias",
			args: []string{"comment", "list"},
			code: "invalid_arguments",
			want: []string{"kaneo-cli comment list-task", "kaneo-cli comment --help"},
		},
		{
			name: "comment list with flags is not an alias",
			args: []string{"comment", "list", "--task-id=" + suppliedValue},
			code: "invalid_arguments",
			want: []string{"kaneo-cli comment list-task", "kaneo-cli comment --help"},
		},
		{
			name: "id to task-id",
			args: []string{"comment", "list-task", "--id", suppliedValue},
			code: "invalid_arguments",
			want: []string{" --task-id", "kaneo-cli comment list-task --help"},
		},
		{
			name: "id to task-id attached value",
			args: []string{"comment", "create-task", "--id=" + suppliedValue, "--body-file", "-"},
			code: "invalid_arguments",
			want: []string{" --task-id", "kaneo-cli comment create-task --help"},
		},
		{
			name: "task-id to id",
			args: []string{"task", "get", "--task-id=" + suppliedValue},
			code: "invalid_arguments",
			want: []string{" --id", "kaneo-cli task get --help"},
		},
		{
			name: "other unknown long flag",
			args: []string{"task", "get", "--unrecognized=" + suppliedValue},
			code: "invalid_arguments",
			want: []string{"kaneo-cli task get --help"},
		},
		{
			name: "unknown shorthand with attached value",
			args: []string{"task", "get", "-Z" + suppliedValue},
			code: "invalid_arguments",
			want: []string{"kaneo-cli task get --help"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
			env := authedEnv(t, server)
			env.setStdin(`{"comment":"must not be submitted"}`)
			updateRequests := 0
			env.app.updateClient = &http.Client{Transport: updateTransport(func(_ *http.Request) (*http.Response, error) {
				updateRequests++
				return nil, errors.New("unexpected update request")
			})}

			status, stdout, stderr := env.run(tt.args...)
			if status != 2 || stdout != "" {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			var response errorResponse
			// Unmarshal rejects any extra JSON object, warning, or help output.
			if err := json.Unmarshal([]byte(stderr), &response); err != nil {
				t.Fatalf("stderr is not one JSON error: %v; stderr=%q", err, stderr)
			}
			if response.Error.Code != tt.code {
				t.Fatalf("error code=%q, want %q", response.Error.Code, tt.code)
			}
			for _, want := range tt.want {
				if !strings.Contains(response.Error.Message, want) {
					t.Errorf("message=%q, want recovery containing %q", response.Error.Message, want)
				}
			}
			if strings.Contains(stderr, suppliedValue) {
				t.Errorf("diagnostic exposed supplied flag value: %q", stderr)
			}
			if len(*requests) != 0 || updateRequests != 0 {
				t.Fatalf("API requests=%d update requests=%d, want none", len(*requests), updateRequests)
			}
		})
	}
}

func TestUsageRecoveryLeavesAPIErrorsUnchanged(t *testing.T) {
	server, requests := bodyServer(t, http.StatusNotFound, "application/json", `{"message":"unknown flag: --server-option","code":"not_found"}`)
	env := authedEnv(t, server)
	status, stdout, stderr := env.run("task", "get", "--id", "t1")
	if status != 5 || stdout != "" || len(*requests) != 1 {
		t.Fatalf("status=%d stdout=%q stderr=%q requests=%d", status, stdout, stderr, len(*requests))
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	var response errorResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatalf("stderr does not end in one JSON error: %v; stderr=%q", err, stderr)
	}
	if response.Error.Code != "not_found" || response.Error.Status != http.StatusNotFound ||
		response.Error.OperationID != "getTask" || strings.Contains(response.Error.Message, "--help") ||
		!strings.Contains(response.Error.Message, "unknown flag: --server-option") {
		t.Fatalf("API diagnostic changed: %+v", response.Error)
	}
}
