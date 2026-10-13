package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPlainTextErrorBodySurfacing covers which failed-response bodies become the
// diagnostic message. Only an exactly text/plain, printable, valid UTF-8 body of
// an operation without a credential-bearing request body is surfaced.
func TestPlainTextErrorBodySurfacing(t *testing.T) {
	const statusMessage = `Invalid status "totally-made-up". Valid statuses for this project: to-do, in-progress, in-review, done, planned, archived`

	tests := []struct {
		name string
		// contentType is left empty to let the test server sniff the body, which
		// is how an unlabeled JSON error arrives as text/plain.
		contentType string
		body        []byte
		secretBody  bool
		want        string
	}{
		{
			name:        "plain text is surfaced",
			contentType: "text/plain;charset=UTF-8",
			body:        []byte(statusMessage),
			want:        statusMessage,
		},
		{
			name:        "secret body is not surfaced",
			contentType: "text/plain;charset=UTF-8",
			body:        []byte(statusMessage),
			secretBody:  true,
			want:        "request failed with HTTP status 400",
		},
		{
			name:        "json body still uses json extraction",
			contentType: "application/json",
			body:        []byte(`{"message":"Invalid key"}`),
			want:        "Invalid key",
		},
		{
			name:        "non text/plain media type is not surfaced",
			contentType: "application/octet-stream",
			body:        []byte(statusMessage),
			want:        "request failed with HTTP status 400",
		},
		{
			name:        "invalid utf-8 is not surfaced",
			contentType: "text/plain",
			body:        []byte{'b', 'a', 'd', 0xff, 0xfe},
			want:        "request failed with HTTP status 400",
		},
		{
			name:        "control character is not surfaced",
			contentType: "text/plain",
			body:        []byte("bad\x00status"),
			want:        "request failed with HTTP status 400",
		},
		{
			name: "sniffed json of an unrecognized shape is not surfaced",
			body: []byte(`{"unexpected":"synthetic-secret-value"}`),
			want: "request failed with HTTP status 400",
		},
		{
			name: "sniffed json with a documented message still extracts it",
			body: []byte(`{"message":"Invalid key"}`),
			want: "Invalid key",
		},
		{
			name:        "bare json scalar is not surfaced",
			contentType: "text/plain; charset=utf-8",
			body:        []byte(`"just a string"`),
			want:        "request failed with HTTP status 400",
		},
		{
			name:        "empty body is not surfaced",
			contentType: "text/plain",
			body:        []byte("   \n  "),
			want:        "request failed with HTTP status 400",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.contentType != "" {
					w.Header().Set("Content-Type", test.contentType)
				}
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(test.body)
			}))
			defer server.Close()

			c := newTestClient(t, server.URL, "x")
			_, err := c.Do(context.Background(), Request{
				Method: "PUT", Path: "/task/status/1", OperationID: "updateTaskStatus",
				SecretBody: test.secretBody,
			})
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *Error", err)
			}
			if apiErr.Error() != test.want {
				t.Fatalf("error = %q, want %q", apiErr.Error(), test.want)
			}
			if strings.Contains(apiErr.Error(), "synthetic-secret-value") {
				t.Fatalf("error leaked an undocumented body value: %q", apiErr.Error())
			}
		})
	}
}

func TestPlainTextErrorBodyIsTruncated(t *testing.T) {
	body := strings.Repeat("a", 500)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "x")
	_, err := c.Do(context.Background(), Request{Method: "PUT", Path: "/x", OperationID: "updateTaskStatus"})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *Error", err)
	}
	if want := strings.Repeat("a", 400) + "..."; apiErr.Message != want {
		t.Fatalf("message = %q, want %q", apiErr.Message, want)
	}
}

func TestPlainTextHTMLBodyKeepsInvalidJSONHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("<html>routing-secret</html>"))
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "x")
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "/x", OperationID: "getX"})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *Error", err)
	}
	if !strings.Contains(apiErr.Message, ErrInvalidJSONResponse.Error()) {
		t.Fatalf("message = %q", apiErr.Message)
	}
	if strings.Contains(apiErr.Message, "routing-secret") {
		t.Fatalf("message leaked body: %q", apiErr.Message)
	}
}

func TestSecretBodyJSONErrorsDoNotReflectCredentials(t *testing.T) {
	const secret = "synthetic-submitted-credential"
	bodies := []string{
		`{"message":"` + secret + `","code":"` + secret + `"}`,
		`{"message":{"description":{"message":"` + secret + `"}},"code":{"code":"` + secret + `"}}`,
		`{"error_description":"` + secret + `","error":"` + secret + `"}`,
		`{"detail":{"code":"` + secret + `"},"error":{"description":"` + secret + `"}}`,
		`{"message":"` + secret + `","error":"authorization_pending"}`,
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusBadGateway} {
		for _, body := range bodies {
			t.Run(http.StatusText(status)+"/"+body, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = w.Write([]byte(body))
				}))
				defer server.Close()

				c := newTestClient(t, server.URL, "x")
				_, err := c.Do(context.Background(), Request{
					Method: "POST", Path: "/credential", OperationID: "credentialRequest",
					Body: []byte(`{"password":"` + secret + `"}`), SecretBody: true,
				})
				var apiErr *Error
				if !errors.As(err, &apiErr) {
					t.Fatalf("error = %v, want *Error", err)
				}
				if apiErr.Message != "" {
					t.Errorf("secret-body message = %q, want empty", apiErr.Message)
				}
				if apiErr.ServerCode != "" {
					t.Errorf("secret-body server code = %q, want empty", apiErr.ServerCode)
				}
				if strings.Contains(apiErr.Error(), secret) {
					t.Errorf("error leaked submitted credential: %q", apiErr.Error())
				}
			})
		}
	}
}

func TestSecretBodyDeviceProtocolCodes(t *testing.T) {
	for _, code := range []string{
		"authorization_pending", "slow_down", "access_denied", "expired_token", "invalid_client",
		"synthetic-submitted-credential", "authorization_pending synthetic-submitted-credential",
	} {
		for _, body := range []string{
			`{"error":"` + code + `","error_description":"synthetic-submitted-credential"}`,
			`{"code":{"code":"` + code + `"},"message":{"message":"synthetic-submitted-credential"}}`,
		} {
			for _, status := range []int{http.StatusBadRequest, http.StatusBadGateway} {
				t.Run(http.StatusText(status)+"/"+body, func(t *testing.T) {
					recorder := httptest.NewRecorder()
					recorder.Header().Set("Content-Type", "application/json")
					recorder.WriteHeader(status)
					_, _ = recorder.WriteString(body)
					resp := recorder.Result()
					defer func() { _ = resp.Body.Close() }()

					apiErr := newError(resp, "deviceTokenPoll", true)
					wantCode := code
					if strings.Contains(code, "synthetic-submitted-credential") {
						wantCode = ""
					}
					if apiErr.ServerCode != wantCode {
						t.Errorf("server code = %q, want %q", apiErr.ServerCode, wantCode)
					}
					if apiErr.Message != "" {
						t.Errorf("secret-body message = %q, want empty", apiErr.Message)
					}
					if strings.Contains(apiErr.Error(), "synthetic-submitted-credential") {
						t.Errorf("error leaked submitted credential: %q", apiErr.Error())
					}
				})
			}
		}
	}
}

func TestOrdinaryJSONErrorsKeepDiagnostics(t *testing.T) {
	for _, body := range []string{
		`{"message":"ordinary diagnostic","code":"ordinary_code"}`,
		`{"message":{"description":"ordinary diagnostic"},"error":{"code":"ordinary_code"}}`,
	} {
		for _, status := range []int{http.StatusBadRequest, http.StatusBadGateway} {
			t.Run(http.StatusText(status)+"/"+body, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				recorder.Header().Set("Content-Type", "application/json")
				recorder.WriteHeader(status)
				_, _ = recorder.WriteString(body)
				resp := recorder.Result()
				defer func() { _ = resp.Body.Close() }()

				apiErr := newError(resp, "ordinaryRequest", false)
				if apiErr.Message != "ordinary diagnostic" || apiErr.ServerCode != "ordinary_code" {
					t.Fatalf("ordinary error = %#v", apiErr)
				}
				if apiErr.Code != statusCode(status) || apiErr.StatusCode != status {
					t.Fatalf("status classification changed: %#v", apiErr)
				}
			})
		}
	}
}
