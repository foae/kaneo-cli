package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/foae/kaneo-cli/internal/buildinfo"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func releaseTestClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := newUpdateClient()
	client.Transport = updateTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != latestReleaseAPI || req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
			t.Errorf("update request must use the public release endpoint without credentials")
		}
		local := req.Clone(req.Context())
		local.URL.Scheme = target.Scheme
		local.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(local)
	})
	return client
}

func TestUpdateNoticeHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}, {"help", "task"}, {"task", "--help"}, {"version"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env := newTestEnv(t)
			env.app.info.Version = "1.0.0"
			env.env["KANEO_TOKEN"] = "synthetic-secret"
			env.app.updateClient = releaseTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, `{"tag_name":"v1.1.0"}`)
			})
			status, stdout, stderr := env.run(args...)
			if status != 0 || strings.Count(stderr, "Update available:") != 1 || !strings.Contains(stderr, "1.0.0 -> v1.1.0") || !strings.Contains(stderr, latestReleasePage) {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
				var info buildinfo.Info
				if err := json.Unmarshal([]byte(stdout), &info); err != nil || info != env.app.info {
					t.Fatalf("version JSON changed: %q, %v", stdout, err)
				}
			} else if !strings.Contains(stdout, "Usage:") || strings.Contains(stdout, "Update available:") {
				t.Fatalf("help output changed: %q", stdout)
			}
		})
	}
}

func TestUpdateNoticeReleaseBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name    string
		current string
		body    string
		status  int
		want    bool
	}{
		{name: "same normalized version", current: "1.0.0", body: `{"tag_name":"v1.0.0"}`},
		{name: "newer installed", current: "v2.0.0", body: `{"tag_name":"v1.9.9"}`},
		{name: "numeric comparison", current: "v1.9.0", body: `{"tag_name":"v1.10.0"}`, want: true},
		{name: "draft", current: "v1.0.0", body: `{"tag_name":"v2.0.0","draft":true}`},
		{name: "prerelease", current: "v1.0.0", body: `{"tag_name":"v2.0.0","prerelease":true}`},
		{name: "prerelease tag", current: "v1.0.0", body: `{"tag_name":"v2.0.0-rc.1"}`},
		{name: "untrusted tag", current: "v1.0.0", body: `{"tag_name":"v2.0.0\nunsafe"}`},
		{name: "malformed response", current: "v1.0.0", body: `{"tag_name":`},
		{name: "oversized response", current: "v1.0.0", body: `{"tag_name":"v2.0.0","body":"` + strings.Repeat("x", 64*1024) + `"}`},
		{name: "rate limited", current: "v1.0.0", status: http.StatusForbidden, body: `{"tag_name":"v2.0.0"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.app.info.Version = tt.current
			env.app.updateClient = releaseTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				_, _ = fmt.Fprint(w, tt.body)
			})
			status, _, stderr := env.run("version")
			if status != 0 || (stderr != "") != tt.want {
				t.Fatalf("status=%d stderr=%q, want notice=%v", status, stderr, tt.want)
			}
		})
	}
}

func TestUpdateCheckSkipsDevelopmentAndOtherCommands(t *testing.T) {
	for _, tt := range []struct {
		version string
		args    []string
	}{
		{version: "dev", args: []string{"version"}},
		{version: "v1.0.1-0.20260921105651-61acf5e2cc44+dirty", args: []string{"--help"}},
		{version: "v2.0.0-rc.1", args: []string{"version"}},
		{version: "v1.0.0", args: []string{"profile", "list"}},
		{version: "v1.0.0", args: []string{"version", "extra"}},
	} {
		t.Run(tt.version+strings.Join(tt.args, " "), func(t *testing.T) {
			env := newTestEnv(t)
			env.app.info.Version = tt.version
			env.app.updateClient = releaseTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("unexpected update check")
			})
			env.run(tt.args...)
		})
	}
}

func TestUpdateCheckTimeoutAndCancellationStaySilent(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelParent), func(t *testing.T) {
			env := newTestEnv(t)
			env.app.updateClient = releaseTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done()
			})
			ctx := t.Context()
			if cancelParent {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			start := time.Now()
			status := executeContext(ctx, env.app, []string{"version"})
			if status != 0 || env.stderr.Len() != 0 || !json.Valid(env.stdout.Bytes()) {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, env.stdout.String(), env.stderr.String())
			}
			if time.Since(start) > 3*time.Second {
				t.Fatal("update check exceeded the short timeout")
			}
		})
	}
}

func TestUpdateCheckDoesNotFollowRedirects(t *testing.T) {
	env := newTestEnv(t)
	env.app.updateClient = releaseTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://untrusted.example/releases/latest", http.StatusFound)
	})
	status, _, stderr := env.run("version")
	if status != 0 || stderr != "" {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
}
