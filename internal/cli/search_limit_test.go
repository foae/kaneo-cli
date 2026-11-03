package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchLimitBoundaries(t *testing.T) {
	for _, test := range []struct {
		name  string
		limit string
		omit  bool
		valid bool
	}{
		{name: "omitted", omit: true, valid: true},
		{name: "minimum", limit: "1", valid: true},
		{name: "maximum", limit: "50", valid: true},
		{name: "below minimum", limit: "0"},
		{name: "above maximum", limit: "51"},
		{name: "negative", limit: "-1"},
		{name: "fractional", limit: "1.5"},
		{name: "empty", limit: ""},
		{name: "nonnumeric", limit: "synthetic-private-value"},
		{name: "overflow", limit: "999999999999999999999999999999"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			var limit string
			var hasLimit bool
			const response = `{"results":[],"totalCount":73}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				limit = r.URL.Query().Get("limit")
				hasLimit = r.URL.Query().Has("limit")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			env := newTestEnv(t)
			env.setAPIURL(server.URL + "/api")
			env.env["KANEO_TOKEN"] = "synthetic"
			args := []string{"search", "global", "--q", "work", "--workspace-id", "W"}
			if !test.omit {
				args = append(args, "--limit", test.limit)
			}
			status, stdout, stderr := env.run(args...)
			if test.valid {
				if status != 0 || requests != 1 || hasLimit == test.omit || limit != test.limit || stdout != response+"\n" {
					t.Fatalf("status=%d requests=%d hasLimit=%t limit=%q stdout=%q stderr=%q", status, requests, hasLimit, limit, stdout, stderr)
				}
				return
			}
			if status != 2 || requests != 0 || stdout != "" {
				t.Fatalf("status=%d requests=%d stdout=%q stderr=%q", status, requests, stdout, stderr)
			}
			var failure errorResponse
			if err := json.Unmarshal([]byte(stderr), &failure); err != nil {
				t.Fatalf("invalid error JSON: %v", err)
			}
			if failure.Error.Code != "invalid_arguments" || !strings.Contains(failure.Error.Message, "--limit") || strings.Contains(stderr, "synthetic-private-value") {
				t.Fatalf("unsafe or unactionable error: %s", stderr)
			}
		})
	}
}
