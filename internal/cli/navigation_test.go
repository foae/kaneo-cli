package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/foae/kaneo-cli/internal/auth"
)

func TestDeviceAuthorizationPagePrintsNavigationURLWithoutNetworkOrCredential(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	env := newTestEnv(t)
	env.setAPIURL(server.URL + "/api")
	env.keyring.err = errors.New("credential backend must not be consulted")
	env.app.deviceLogin = func(_ context.Context, _ auth.DeviceOptions) (auth.DeviceToken, error) {
		t.Fatal("navigation command started device login")
		return auth.DeviceToken{}, nil
	}

	status, stdout, stderr := env.run("auth", "get-device-authorization-page", "--user-code", "AB C&1")
	if status != 0 {
		t.Fatalf("status=%d stderr=%q", status, stderr)
	}
	want := server.URL + "/api/auth/device?ui=1&user_code=AB+C%261\n"
	if stdout != want {
		t.Fatalf("stdout=%q want %q", stdout, want)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests=%d, want 0", requests.Load())
	}
	if !strings.Contains(stderr, "only prints the authorization URL") || !strings.Contains(stderr, "browser cookies may require HTTPS") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestMCPAuthorizationHandoffEncodesAndPreservesStateOmission(t *testing.T) {
	args := []string{
		"mcp", "start-authorization",
		"--response-type", "code",
		"--client-id", "client & one",
		"--redirect-uri", "https://client.example/callback?x=a&y=b",
		"--code-challenge", "challenge+/=",
		"--code-challenge-method", "S256",
	}
	for _, test := range []struct {
		name      string
		args      []string
		wantQuery url.Values
	}{
		{
			name: "state omitted",
			args: args,
			wantQuery: url.Values{
				"response_type":         {"code"},
				"client_id":             {"client & one"},
				"redirect_uri":          {"https://client.example/callback?x=a&y=b"},
				"code_challenge":        {"challenge+/="},
				"code_challenge_method": {"S256"},
			},
		},
		{
			name: "state explicitly empty",
			args: append(append([]string{}, args...), "--state", ""),
			wantQuery: url.Values{
				"response_type":         {"code"},
				"client_id":             {"client & one"},
				"redirect_uri":          {"https://client.example/callback?x=a&y=b"},
				"code_challenge":        {"challenge+/="},
				"code_challenge_method": {"S256"},
				"state":                 {""},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.setAPIURL("https://kaneo.example/api")
			env.keyring.err = errors.New("credential backend must not be consulted")

			status, stdout, stderr := env.run(test.args...)
			if status != 0 {
				t.Fatalf("status=%d stderr=%q", status, stderr)
			}
			parsed, err := url.Parse(strings.TrimSuffix(stdout, "\n"))
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Scheme != "https" || parsed.Host != "kaneo.example" || parsed.Path != "/api/mcp/authorize" {
				t.Fatalf("URL=%q", stdout)
			}
			if got := parsed.Query(); !reflect.DeepEqual(got, test.wantQuery) {
				t.Fatalf("query=%q want %q", got.Encode(), test.wantQuery.Encode())
			}
			if strings.Count(stdout, "\n") != 1 || !strings.Contains(stderr, "only prints the authorization URL") {
				t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
			}
		})
	}
}

func TestNavigationRejectsInvalidInputBeforeOutput(t *testing.T) {
	env := newTestEnv(t)
	env.setAPIURL("https://kaneo.example/api")

	for _, args := range [][]string{
		{"auth", "get-device-authorization-page", "--ui", "0"},
		{"mcp", "start-authorization", "--response-type", "token"},
		{"mcp", "start-authorization", "--response-type", "code", "--client-id", "client", "--redirect-uri", strings.Repeat("x", 2049), "--code-challenge", "challenge", "--code-challenge-method", "S256"},
	} {
		status, stdout, _ := env.run(args...)
		if status != 2 || stdout != "" {
			t.Fatalf("%v: status=%d stdout=%q", args, status, stdout)
		}
	}
}
