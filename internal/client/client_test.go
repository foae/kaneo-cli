package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func newTestClient(t *testing.T, rawURL, token string) *Client {
	t.Helper()
	base, err := ParseBaseURL(rawURL)
	if err != nil {
		t.Fatalf("ParseBaseURL(%q) error: %v", rawURL, err)
	}
	c, err := New(Options{BaseURL: base, Token: token, WarnWriter: io.Discard})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return c
}

func TestParseBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "normalizes default port and slash", raw: "HTTPS://Cloud.Kaneo.App:443/api/", want: "https://cloud.kaneo.app/api"},
		{name: "keeps non-default port", raw: "http://localhost:8080/api", want: "http://localhost:8080/api"},
		{name: "keeps api path", raw: "https://example.com/api", want: "https://example.com/api"},
		{name: "empty is invalid", raw: "", wantErr: true},
		{name: "userinfo rejected", raw: "https://user:pass@example.com/api", wantErr: true},
		{name: "scheme required", raw: "example.com/api", wantErr: true},
		{name: "query rejected", raw: "https://example.com/api?x=1", wantErr: true},
		{name: "fragment rejected", raw: "https://example.com/api#frag", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBaseURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseBaseURL(%q) = %q, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBaseURL(%q) error: %v", tt.raw, err)
			}
			if got.String() != tt.want {
				t.Fatalf("ParseBaseURL(%q) = %q, want %q", tt.raw, got.String(), tt.want)
			}
		})
	}
}

func TestJoinPath(t *testing.T) {
	base, err := ParseBaseURL("https://example.com/api")
	if err != nil {
		t.Fatal(err)
	}
	joined, err := base.Join("/instance/status")
	if err != nil {
		t.Fatal(err)
	}
	if joined.String() != "https://example.com/api/instance/status" {
		t.Fatalf("Join() = %q", joined.String())
	}
}

func TestDoPreservesRequestAndResponse(t *testing.T) {
	var got struct {
		method      string
		path        string
		query       url.Values
		auth        string
		contentType string
		body        []byte
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.method = r.Method
		got.path = r.URL.Path
		got.query = r.URL.Query()
		got.auth = r.Header.Get("Authorization")
		got.contentType = r.Header.Get("Content-Type")
		got.body = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"n":9007199254740993,"unknown":{"x":1}}`))
	}))
	defer server.Close()

	c := newTestClient(t, server.URL+"/api", "synthetic-token")
	resp, err := c.Do(context.Background(), Request{
		Method:      "POST",
		Path:        "/task",
		Query:       url.Values{"workspaceId": {"ws1"}},
		Body:        []byte(`{"title":"x"}`),
		ContentType: "application/json",
	})
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if got.method != "POST" || got.path != "/api/task" {
		t.Fatalf("request = %s %s", got.method, got.path)
	}
	if got.query.Get("workspaceId") != "ws1" {
		t.Fatalf("query = %v", got.query)
	}
	if got.auth != "Bearer synthetic-token" {
		t.Fatalf("Authorization = %q", got.auth)
	}
	if got.contentType != "application/json" {
		t.Fatalf("Content-Type = %q", got.contentType)
	}
	if !bytes.Equal(got.body, []byte(`{"title":"x"}`)) {
		t.Fatalf("body = %s", got.body)
	}
	if string(raw) != `{"n":9007199254740993,"unknown":{"x":1}}` {
		t.Fatalf("response body = %s", raw)
	}
}

func TestDoTranslatesHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized","message":"Invalid key"}`))
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "x")
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "/auth/get-session", OperationID: "getSession"})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *Error", err)
	}
	if apiErr.StatusCode != 401 || apiErr.Code != "authentication_failed" {
		t.Fatalf("error = %+v", apiErr)
	}
	if apiErr.Message != "Invalid key" {
		t.Fatalf("message = %q", apiErr.Message)
	}
	if apiErr.ServerCode != "unauthorized" {
		t.Fatalf("server code = %q", apiErr.ServerCode)
	}
	if apiErr.RetryAfter != 7 {
		t.Fatalf("retry after = %d", apiErr.RetryAfter)
	}
	if apiErr.OperationID != "getSession" {
		t.Fatalf("operation id = %q", apiErr.OperationID)
	}
}

func TestDoEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "")
	resp, err := c.Do(context.Background(), Request{Method: "DELETE", Path: "/x"})
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if !resp.Empty() {
		t.Fatal("Empty() = false, want true for 204")
	}
}

func TestDoRefusesCredentialedCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/elsewhere", http.StatusFound)
	}))
	defer source.Close()

	c := newTestClient(t, source.URL, "synthetic-token")
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	if !IsRedirectRefusal(err) {
		t.Fatalf("error = %v, want redirect refusal", err)
	}
}

func TestDoFollowsUnauthenticatedCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/elsewhere", http.StatusFound)
	}))
	defer source.Close()

	c := newTestClient(t, source.URL, "")
	resp, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if string(raw) != `{"ok":true}` {
		t.Fatalf("body = %s", raw)
	}
}

func TestDoTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	base, _ := ParseBaseURL(server.URL)
	c, err := New(Options{BaseURL: base, HTTPClient: &http.Client{Timeout: 30 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(context.Background(), Request{Method: "GET", Path: "/slow"})
	if !IsTimeout(err) {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestDoCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	_, err := c.Do(ctx, Request{Method: "GET", Path: "/slow"})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestPlainHTTPWarningOnlyWithCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	var warned bytes.Buffer
	base, _ := ParseBaseURL(server.URL)
	c, err := New(Options{BaseURL: base, Token: "synthetic", WarnWriter: &warned})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Authenticated() {
		t.Fatal("Authenticated() = false")
	}
	resp, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if warned.Len() == 0 {
		t.Fatal("no plain-HTTP warning emitted")
	}

	warned.Reset()
	unauthenticated := newTestClient(t, server.URL, "")
	unauthenticated.warn = &warned
	resp, err = unauthenticated.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if warned.Len() != 0 {
		t.Fatalf("unauthenticated request warned: %q", warned.String())
	}
}

func TestErrorBodyIsBoundedAndNotEchoed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"unexpected":"synthetic-secret-value"}`))
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "")
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v", err)
	}
	if apiErr.Message != "" {
		t.Fatalf("message = %q, want no unrecognized body echoed", apiErr.Message)
	}
}

func TestTransportFailureIsTyped(t *testing.T) {
	// A closed port produces a connection-refused transport error.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rawURL := server.URL
	server.Close()

	c := newTestClient(t, rawURL, "")
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x"})
	var transport *TransportError
	if !errors.As(err, &transport) {
		t.Fatalf("error = %v, want *TransportError", err)
	}
	if transport.Error() != "transport failure" {
		t.Fatalf("message = %q", transport.Error())
	}
}

func TestSensitiveCrossOriginBodyRedirectRefused(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL+"/elsewhere")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	c := newTestClient(t, source.URL, "")
	_, err := c.Do(context.Background(), Request{
		Method:    "POST",
		Path:      "/auth/device/token",
		Body:      []byte(`{"device_code":"secret"}`),
		Sensitive: true,
	})
	if !IsRedirectRefusal(err) {
		t.Fatalf("error = %v, want redirect refusal for a sensitive body", err)
	}
}

func TestIPv6BaseURLKeepsBrackets(t *testing.T) {
	got, err := ParseBaseURL("http://[::1]/api")
	if err != nil {
		t.Fatalf("ParseBaseURL() error: %v", err)
	}
	if got.String() != "http://[::1]/api" {
		t.Fatalf("ParseBaseURL() = %q", got.String())
	}
	if got.Origin() != "http://[::1]" {
		t.Fatalf("Origin() = %q", got.Origin())
	}
}

func TestParseBaseURLJSONRoundTrip(t *testing.T) {
	base, err := ParseBaseURL("https://example.com/api")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(base.String())
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"https://example.com/api"` {
		t.Fatalf("encoded = %s", encoded)
	}
}
