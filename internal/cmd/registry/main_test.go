package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestVerifyContainerIdentity(t *testing.T) {
	for _, scenario := range []string{"complete", "wrong revision", "missing arm64", "corrupt blob"} {
		t.Run(scenario, func(t *testing.T) {
			objects := map[string][]byte{}
			put := func(value any) string {
				data, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
				objects[digest] = data
				return digest
			}
			var index manifest
			for _, arch := range []string{"amd64", "arm64"} {
				if scenario == "missing arm64" && arch == "arm64" {
					continue
				}
				var config imageConfig
				config.OS, config.Architecture = "linux", arch
				config.Config.User = "65532:65532"
				config.Config.Entrypoint = []string{"/usr/local/bin/kaneo-cli"}
				config.Config.Labels = map[string]string{
					"org.opencontainers.image.source":   "https://github.com/foae/kaneo-cli",
					"org.opencontainers.image.revision": "verified-commit",
					"org.opencontainers.image.version":  "1.2.0",
					"org.opencontainers.image.licenses": "MIT",
				}
				if scenario == "wrong revision" {
					config.Config.Labels["org.opencontainers.image.revision"] = "another-commit"
				}
				var image manifest
				image.Config.Digest = put(config)
				if scenario == "corrupt blob" {
					objects[image.Config.Digest] = []byte(`{}`)
				}
				entry := descriptor{Digest: put(image)}
				entry.Platform.OS, entry.Platform.Architecture = "linux", arch
				index.Manifests = append(index.Manifests, entry)
			}
			indexDigest := put(index)
			r := registry{client: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				digest := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(objects[digest])), Header: make(http.Header)}, nil
			})}}
			err := r.verify(context.Background(), objects[indexDigest], "v1.2.0", "verified-commit")
			if (err == nil) != (scenario == "complete") {
				t.Fatalf("verify returned %v", err)
			}
		})
	}
}

func TestRedirectDoesNotForwardRegistryCredential(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://blob.example.test/object", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer synthetic-registry-secret")
	if err := registryRedirect(req, nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("registry credential forwarded to blob storage")
	}
	req.URL.Scheme = "http"
	if err := registryRedirect(req, nil); err == nil {
		t.Fatal("accepted insecure redirect")
	}
}

func TestInspectionFailsClosed(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	for _, tc := range []struct {
		name       string
		args       []string
		authStatus int
		authBody   string
		status     int
		wantError  bool
	}{
		{"absent", []string{"absent", "v1.2.0"}, 200, `{"token":"synthetic"}`, 404, false},
		{"existing", []string{"absent", "v1.2.0"}, 200, `{"token":"synthetic"}`, 200, true},
		{"forbidden", []string{"absent", "v1.2.0"}, 200, `{"token":"synthetic"}`, 403, true},
		{"registry failure", []string{"absent", "v1.2.0"}, 200, `{"token":"synthetic"}`, 500, true},
		{"auth failure", []string{"absent", "v1.2.0"}, 403, `{}`, 404, true},
		{"empty auth", []string{"absent", "v1.2.0"}, 200, `{}`, 404, true},
		{"wrong digest", []string{"verify", "v1.2.0", "commit", "sha256:" + strings.Repeat("0", 64)}, 200, `{"token":"synthetic"}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := registry{client: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				status, body := tc.status, `{}`
				if req.URL.Path == "/token" {
					status, body = tc.authStatus, tc.authBody
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}}
			err := r.inspect(context.Background(), tc.args)
			if (err != nil) != tc.wantError {
				t.Fatalf("inspection returned %v; want error: %v", err, tc.wantError)
			}
		})
	}
}

func TestVerificationUsesReadOnlyScope(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "synthetic")
	t.Setenv("GITHUB_ACTOR", "synthetic-user")
	r := registry{client: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if req.URL.Query().Get("scope") != "repository:foae/kaneo-cli:pull" {
			status = http.StatusForbidden
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"token":"synthetic"}`)), Header: make(http.Header)}, nil
	})}}
	if err := r.authenticate(context.Background(), false); err != nil {
		t.Fatalf("read-only registry authorization rejected: %v", err)
	}
}
