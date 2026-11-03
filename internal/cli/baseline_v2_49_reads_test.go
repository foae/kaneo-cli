package cli

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBaselineV249ReadParametersRejectBeforeSession(t *testing.T) {
	for _, args := range [][]string{
		{"admin", "list-users", "--status", "disabled"},
		{"admin", "list-users", "--email-verified", "TRUE"},
		{"admin", "list-users", "--email-verified", ""},
		{"admin", "list-users", "--role", ""},
		{"admin", "list-users", "--role", strings.Repeat("é", 65)},
		{"admin", "list-audit-log", "--action", ""},
		{"admin", "list-audit-log", "--action", "user.*"},
		{"admin", "list-audit-log", "--action", "user.created.extra"},
		{"admin", "list-audit-log", "--action", "User"},
		{"admin", "list-audit-log", "--action", strings.Repeat("a", 101)},
		{"admin", "list-audit-log", "--actor-id", strings.Repeat("é", 201)},
		{"admin", "list-audit-log", "--target-id", strings.Repeat("a", 201)},
		{"admin", "list-audit-log", "--target-type", strings.Repeat("a", 101)},
		{"admin", "list-audit-log", "--page", "0"},
		{"admin", "list-audit-log", "--page", "1000001"},
		{"admin", "list-audit-log", "--page", "99999999999999999999"},
		{"admin", "list-audit-log", "--limit", "0"},
		{"admin", "list-audit-log", "--limit", "101"},
		{"admin", "list-audit-log", "--limit", "1.5"},
		{"admin", "list-user-sessions"},
		{"admin", "list-user-sessions", "--user-id", ""},
		{"admin", "list-user-sessions", "--user-id", strings.Repeat("a", 201)},
		{"admin", "list-user-api-keys", "--user-id", ".."},
		{"admin", "list-user-api-keys", "--user-id", strings.Repeat("é", 201)},
		{"admin", "list-webhook-deliveries", "--webhook-id", ""},
		{"task-relation", "list-project", "--project-id", "."},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.app.configDir = func() (string, error) {
				t.Fatal("invalid parameters reached session or credential setup")
				return "", nil
			}
			status, stdout, stderr := env.run(args...)
			if status != 2 || stdout != "" || !strings.Contains(stderr, "invalid_arguments") || len(*requests) != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%+v", status, stdout, stderr, *requests)
			}
		})
	}
}

// These fixtures exercise CLI validation boundaries, not upstream filtering.
// The enums, action grammar and lengths also match apps/api/src/admin/schema.ts
// at e0ebc61748654f4e33e713dcb441be726ec1cfc4.
func TestBaselineV249ReadFilterBoundaries(t *testing.T) {
	for _, tt := range []struct {
		args   []string
		target string
	}{
		{
			[]string{"admin", "list-users", "--status", "deactivated", "--email-verified", "false", "--role", strings.Repeat("é", 64)},
			"/api/admin/users?emailVerified=false&role=" + url.QueryEscape(strings.Repeat("é", 64)) + "&status=deactivated",
		},
		{
			[]string{"admin", "list-users", "--status", "active", "--email-verified", "true", "--role", "user"},
			"/api/admin/users?emailVerified=true&role=user&status=active",
		},
		{
			[]string{"admin", "list-audit-log", "--action", "user", "--page", "1000000", "--limit", "1"},
			"/api/admin/audit-log?action=user&limit=1&page=1000000",
		},
		{
			[]string{"admin", "list-audit-log", "--action", "user.role_changed", "--actor-id", strings.Repeat("a", 200), "--target-type", strings.Repeat("a", 100), "--target-id", "", "--page", "1", "--limit", "100"},
			"/api/admin/audit-log?action=user.role_changed&actorId=" + strings.Repeat("a", 200) + "&limit=100&page=1&targetId=&targetType=" + strings.Repeat("a", 100),
		},
	} {
		t.Run(strings.Join(tt.args[:2], " ")+" "+tt.target, func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusOK, "application/json", `{}`)
			env := authedEnv(t, server)
			if status, _, stderr := env.run(tt.args...); status != 0 {
				t.Fatalf("status=%d stderr=%q", status, stderr)
			}
			if len(*requests) != 1 || (*requests)[0].target != tt.target {
				t.Fatalf("requests=%+v, want %s", *requests, tt.target)
			}
		})
	}
}

func TestBaselineV249ReadPermissionFailuresDoNotRetry(t *testing.T) {
	for _, args := range [][]string{
		{"admin", "get-overview"},
		{"admin", "list-user-sessions", "--user-id", "u1"},
		{"user", "list-sessions"},
		{"task-relation", "list-project", "--project-id", "p1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			server, requests := bodyServer(t, http.StatusForbidden, "application/json", `{"message":"Missing permission or authenticated with an API key"}`)
			env := authedEnv(t, server)
			status, stdout, stderr := env.run(args...)
			if status != 3 || stdout != "" || len(*requests) != 1 || !strings.Contains(stderr, `"code":"authorization_failed"`) {
				t.Fatalf("status=%d stdout=%q stderr=%q requests=%+v", status, stdout, stderr, *requests)
			}
			if (*requests)[0].auth != "Bearer synthetic-secret" || strings.Contains(stdout+stderr, "synthetic-secret") {
				t.Fatal("credential was missing from the request or leaked into output")
			}
		})
	}
}
