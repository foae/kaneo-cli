package cli

import (
	"sort"
	"strings"
	"testing"
)

// TestGeneratedSecretBodyOperations pins the generated set of operations whose
// request body carries a credential.
func TestGeneratedSecretBodyOperations(t *testing.T) {
	want := []string{
		"createDiscordIntegration",
		"createGenericWebhookIntegration",
		"createGiteaIntegration",
		"createMattermostIntegration",
		"createSlackIntegration",
		"createTelegramIntegration",
		"listGiteaRepositories",
		"updateDiscordIntegration",
		"updateGenericWebhookIntegration",
		"updateMattermostIntegration",
		"updateNotificationPreferences",
		"updateSlackIntegration",
		"updateTelegramIntegration",
		"verifyGiteaAccess",
	}
	got := make([]string, 0, len(generatedSecretBodyOperations))
	for operationID, secret := range generatedSecretBodyOperations {
		if !secret {
			t.Fatalf("operation %q is present but false", operationID)
		}
		got = append(got, operationID)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("secret body operations = %v, want %v", got, want)
	}
}

// TestStatusHelpNotes checks that the status semantics are visible in help.
func TestStatusHelpNotes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "task update-status",
			args: []string{"task", "update-status", "--help"},
			want: `status is a column slug from 'column list', or one of the reserved values "planned" (the Backlog board) or "archived". Any other value is rejected with HTTP 400 and the task is left unchanged.`,
		},
		{
			name: "task create",
			args: []string{"task", "create", "--help"},
			want: `status is a column slug from 'column list', or the reserved value "planned" or "archived" to file the task straight into the Backlog or Archive.`,
		},
		{
			name: "task move",
			args: []string{"task", "move", "--help"},
			want: `destinationStatus must be a column slug of the destination project. The reserved values "planned" and "archived" are rejected here, unlike 'task update-status'.`,
		},
		{
			name: "task import",
			args: []string{"task", "import", "--help"},
			want: `An unrecognized status is not rejected: the server rewrites it to "planned" and the task lands in the Backlog. The response still reports success, so read results.tasks[].warnings.`,
		},
		{
			name: "task list status flag",
			args: []string{"task", "list", "--help"},
			want: "Column slug, or the reserved values planned or archived",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnv(t)
			status, stdout, stderr := env.run(test.args...)
			if status != 0 {
				t.Fatalf("status = %d, stderr = %s", status, stderr)
			}
			if !strings.Contains(collapseHelp(stdout), collapseHelp(test.want)) {
				t.Fatalf("help = %q, want it to contain %q", stdout, test.want)
			}
		})
	}
}

// collapseHelp normalizes whitespace so wrapped help text still matches.
func collapseHelp(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
