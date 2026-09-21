package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/foae/kaneo-cli/internal/client"
)

type deviceFixture struct {
	mu           sync.Mutex
	code         DeviceCode
	tokenStatus  []int
	tokenBodies  []string
	tokenCalls   int
	codeRequests int
}

func (f *deviceFixture) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/api/auth/device/code":
			f.mu.Lock()
			f.codeRequests++
			f.mu.Unlock()
			var request struct {
				ClientID string `json:"client_id"`
			}
			if err := json.Unmarshal(body, &request); err != nil || request.ClientID == "" {
				t.Errorf("device code request = %s", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(f.code)
		case "/api/auth/device/token":
			f.mu.Lock()
			index := f.tokenCalls
			f.tokenCalls++
			f.mu.Unlock()
			status := http.StatusOK
			if index < len(f.tokenStatus) {
				status = f.tokenStatus[index]
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if index < len(f.tokenBodies) {
				_, _ = w.Write([]byte(f.tokenBodies[index]))
			}
		default:
			http.NotFound(w, r)
		}
	}
}

func newDeviceClient(t *testing.T, server *httptest.Server) *client.Client {
	t.Helper()
	base, err := client.ParseBaseURL(server.URL + "/api")
	if err != nil {
		t.Fatal(err)
	}
	c, err := client.New(client.Options{BaseURL: base, WarnWriter: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoginDevicePendingThenApproved(t *testing.T) {
	fixture := &deviceFixture{
		code: DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1, ExpiresIn: 600},
		tokenStatus: []int{
			http.StatusBadRequest,
			http.StatusOK,
		},
		tokenBodies: []string{
			`{"error":"authorization_pending"}`,
			`{"access_token":"issued-token","token_type":"Bearer"}`,
		},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()
	fixture.code.VerificationURI = server.URL + "/device"

	var instructions bytes.Buffer
	var delays []time.Duration
	token, err := LoginDevice(context.Background(), DeviceOptions{
		Client:       newDeviceClient(t, server),
		Instructions: &instructions,
		Sleep: func(_ context.Context, d time.Duration) error {
			delays = append(delays, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LoginDevice() error: %v", err)
	}
	if token.AccessToken != "issued-token" {
		t.Fatalf("token = %+v", token)
	}
	if len(delays) != 2 {
		t.Fatalf("delays = %v, want two polls", delays)
	}
	output := instructions.String()
	if !strings.Contains(output, "ABCD-1234") {
		t.Fatalf("instructions missing user code: %q", output)
	}
	if strings.Contains(output, "dev_secret") || strings.Contains(output, "issued-token") {
		t.Fatalf("instructions leaked a secret: %q", output)
	}
}

func TestLoginDeviceSlowDownIncreasesInterval(t *testing.T) {
	fixture := &deviceFixture{
		code:        DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1, ExpiresIn: 600},
		tokenStatus: []int{http.StatusBadRequest, http.StatusOK},
		tokenBodies: []string{
			`{"error":"slow_down"}`,
			`{"access_token":"issued-token","token_type":"Bearer"}`,
		},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()
	fixture.code.VerificationURI = server.URL + "/device"

	var delays []time.Duration
	token, err := LoginDevice(context.Background(), DeviceOptions{
		Client: newDeviceClient(t, server),
		Sleep: func(_ context.Context, d time.Duration) error {
			delays = append(delays, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LoginDevice() error: %v", err)
	}
	if token.AccessToken != "issued-token" {
		t.Fatalf("token = %+v", token)
	}
	if len(delays) != 2 || delays[1] <= delays[0] {
		t.Fatalf("delays = %v, want an increased second interval", delays)
	}
}

func TestLoginDeviceTerminalOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		errorOut string
		wantErr  error
	}{
		{name: "denied", errorOut: "access_denied", wantErr: ErrDeviceAccessDenied},
		{name: "expired", errorOut: "expired_token", wantErr: ErrDeviceExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := &deviceFixture{
				code:        DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1, ExpiresIn: 600},
				tokenStatus: []int{http.StatusBadRequest},
				tokenBodies: []string{`{"error":"` + tt.errorOut + `"}`},
			}
			server := httptest.NewServer(fixture.handler(t))
			defer server.Close()
			fixture.code.VerificationURI = server.URL + "/device"

			_, err := LoginDevice(context.Background(), DeviceOptions{
				Client: newDeviceClient(t, server),
				Sleep:  func(context.Context, time.Duration) error { return nil },
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoginDeviceInvalidClient(t *testing.T) {
	fixture := &deviceFixture{
		code:        DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1, ExpiresIn: 600},
		tokenStatus: []int{http.StatusBadRequest},
		tokenBodies: []string{`{"error":"invalid_client"}`},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()
	fixture.code.VerificationURI = server.URL + "/device"

	_, err := LoginDevice(context.Background(), DeviceOptions{
		Client: newDeviceClient(t, server),
		Sleep:  func(context.Context, time.Duration) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "client id") {
		t.Fatalf("error = %v, want invalid-client failure", err)
	}
}

func TestLoginDeviceExpiresWithoutPollingIndefinitely(t *testing.T) {
	fixture := &deviceFixture{
		code: DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1, ExpiresIn: 1},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()
	fixture.code.VerificationURI = server.URL + "/device"

	now := time.Unix(0, 0)
	_, err := LoginDevice(context.Background(), DeviceOptions{
		Client: newDeviceClient(t, server),
		Sleep: func(_ context.Context, d time.Duration) error {
			now = now.Add(d)
			return nil
		},
		Now: func() time.Time { return now },
	})
	if !errors.Is(err, ErrDeviceExpired) {
		t.Fatalf("error = %v, want ErrDeviceExpired", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.tokenCalls != 0 {
		t.Fatalf("polled %d times after expiry", fixture.tokenCalls)
	}
}

func TestLoginDeviceCancellation(t *testing.T) {
	fixture := &deviceFixture{
		code: DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1, ExpiresIn: 600},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()
	fixture.code.VerificationURI = server.URL + "/device"

	ctx, cancel := context.WithCancel(context.Background())
	_, err := LoginDevice(ctx, DeviceOptions{
		Client: newDeviceClient(t, server),
		Sleep: func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestLoginDeviceBoundsHostileInterval(t *testing.T) {
	fixture := &deviceFixture{
		code: DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1 << 40, ExpiresIn: 1},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()
	fixture.code.VerificationURI = server.URL + "/device"

	now := time.Unix(0, 0)
	var delays []time.Duration
	_, err := LoginDevice(context.Background(), DeviceOptions{
		Client: newDeviceClient(t, server),
		Sleep: func(_ context.Context, d time.Duration) error {
			delays = append(delays, d)
			now = now.Add(d)
			return nil
		},
		Now: func() time.Time { return now },
	})
	if !errors.Is(err, ErrDeviceExpired) {
		t.Fatalf("error = %v, want ErrDeviceExpired", err)
	}
	if len(delays) != 1 || delays[0] > time.Second {
		t.Fatalf("delays = %v, want a single bounded wait", delays)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.tokenCalls != 0 {
		t.Fatalf("polled %d times", fixture.tokenCalls)
	}
}

func TestLoginDeviceContinuesOn2xxErrorPayload(t *testing.T) {
	fixture := &deviceFixture{
		code:        DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1, ExpiresIn: 600},
		tokenStatus: []int{http.StatusOK, http.StatusOK},
		tokenBodies: []string{
			`{"error":"authorization_pending"}`,
			`{"access_token":"issued-token","token_type":"Bearer"}`,
		},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()
	fixture.code.VerificationURI = server.URL + "/device"

	token, err := LoginDevice(context.Background(), DeviceOptions{
		Client: newDeviceClient(t, server),
		Sleep:  func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("LoginDevice() error: %v", err)
	}
	if token.AccessToken != "issued-token" {
		t.Fatalf("token = %+v", token)
	}
}

func TestLoginDeviceRejectsUnknownTokenType(t *testing.T) {
	fixture := &deviceFixture{
		code:        DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "", Interval: 1, ExpiresIn: 600},
		tokenStatus: []int{http.StatusOK},
		tokenBodies: []string{`{"access_token":"issued-token","token_type":"MAC"}`},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()
	fixture.code.VerificationURI = server.URL + "/device"

	_, err := LoginDevice(context.Background(), DeviceOptions{
		Client: newDeviceClient(t, server),
		Sleep:  func(context.Context, time.Duration) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "token type") {
		t.Fatalf("error = %v, want unsupported token type", err)
	}
}

func TestSanitizeForDisplayStripsBidi(t *testing.T) {
	got := SanitizeForDisplay("ab\u202ecd\u2066ef\u200f")
	if got != "abcdef" {
		t.Fatalf("SanitizeForDisplay() = %q", got)
	}
}

func TestLoginDeviceRejectsUnsafeVerificationURL(t *testing.T) {
	fixture := &deviceFixture{
		code: DeviceCode{DeviceCode: "dev_secret", UserCode: "ABCD-1234", VerificationURI: "javascript:alert(1)", Interval: 1, ExpiresIn: 600},
	}
	server := httptest.NewServer(fixture.handler(t))
	defer server.Close()

	var instructions bytes.Buffer
	_, err := LoginDevice(context.Background(), DeviceOptions{
		Client:       newDeviceClient(t, server),
		Instructions: &instructions,
		Sleep:        func(context.Context, time.Duration) error { return nil },
	})
	if err == nil {
		t.Fatal("LoginDevice() accepted an unsafe verification URL")
	}
	if strings.Contains(instructions.String(), "javascript") {
		t.Fatalf("instructions leaked an unsafe URL: %q", instructions.String())
	}
}
