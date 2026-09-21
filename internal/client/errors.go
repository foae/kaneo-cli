package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// maxErrorBody bounds how much of a failed response is read and retained.
const maxErrorBody = 64 << 10

// ErrInvalidJSONResponse gives safe, actionable guidance when an endpoint
// expected to return JSON instead returns another document or shape. It intentionally
// excludes the response body and parser details, either of which may expose
// secrets.
var ErrInvalidJSONResponse = errors.New("server returned invalid JSON or an unexpected JSON shape; check server compatibility. If the response is a dashboard or proxy page, verify the API base URL (normally including /api) with `kaneo-cli profile get`, --api-url, and KANEO_API_URL")

// Error is a non-2xx API response translated into stable, safe output.
type Error struct {
	StatusCode  int
	Code        string
	Message     string
	OperationID string
	// ServerCode is the machine-readable code the server returned, for example
	// the RFC 8628 device-flow error names. It is empty when absent.
	ServerCode string
	// RetryAfter is the parsed Retry-After delay in seconds, or zero.
	RetryAfter int
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("request failed with HTTP status %d", e.StatusCode)
}

// RedirectError reports a refused redirect. Only normalized origins are
// retained so a secret-bearing Location URL is never rendered.
type RedirectError struct {
	From string
	To   string
}

func (e *RedirectError) Error() string {
	return fmt.Sprintf("refused redirect from origin %s to %s", e.From, e.To)
}

// Timeout reports an exceeded deadline or client timeout.
type TimeoutError struct {
	Err error
}

func (e *TimeoutError) Error() string { return "request timed out" }

func (e *TimeoutError) Unwrap() error { return e.Err }

// TransportError reports a non-timeout transport failure whose underlying
// error may contain a secret-bearing URL. Its message is intentionally generic.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return "transport failure" }

func (e *TransportError) Unwrap() error { return e.Err }

func statusCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusUnauthorized:
		return "authentication_failed"
	case http.StatusForbidden:
		return "authorization_failed"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusUnprocessableEntity:
		return "unprocessable_entity"
	case http.StatusTooManyRequests:
		return "rate_limited"
	}
	if status >= 500 {
		return "server_error"
	}
	return "api_error"
}

// newError translates a failed response into an *Error, reading a bounded
// portion of the body for a documented message field. It never echoes an
// unrecognized raw body, which could contain reflected secrets.
func newError(resp *http.Response, operationID string) *Error {
	apiErr := &Error{
		StatusCode:  resp.StatusCode,
		Code:        statusCode(resp.StatusCode),
		OperationID: operationID,
		RetryAfter:  parseRetryAfter(resp.Header.Get("Retry-After")),
	}
	if resp.Body == nil {
		return apiErr
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil || len(body) == 0 {
		return apiErr
	}
	if beginsWithHTML(body) {
		apiErr.Message = fmt.Sprintf("request failed with HTTP status %d; %s", resp.StatusCode, ErrInvalidJSONResponse)
		return apiErr
	}
	apiErr.Message = extractMessage(body)
	apiErr.ServerCode = extractServerCode(body)
	return apiErr
}

func beginsWithHTML(body []byte) bool {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(body), []byte("\xef\xbb\xbf")))
	return len(trimmed) > 0 && trimmed[0] == '<'
}

// extractMessage pulls a message from the documented error shapes. Only known
// message fields are used; an unrecognized body yields no message so the generic
// status message is shown instead.
func extractMessage(body []byte) string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"message", "error_description", "error", "detail"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		if message := rawMessage(raw); message != "" {
			return truncate(collapse(message))
		}
	}
	return ""
}

// extractServerCode reads the documented machine-readable error identifier from
// the error payload without retaining unrelated fields.
func extractServerCode(body []byte) string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"error", "code"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		if code := rawMessage(raw); code != "" {
			return strings.TrimSpace(code)
		}
	}
	return ""
}

func rawMessage(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		for _, key := range []string{"message", "description", "code"} {
			if inner, ok := object[key]; ok {
				if message := rawMessage(inner); message != "" {
					return message
				}
			}
		}
	}
	return ""
}

func collapse(message string) string {
	return strings.Join(strings.Fields(message), " ")
}

func truncate(message string) string {
	const limit = 400
	if len(message) <= limit {
		return message
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut] + "..."
}

func parseRetryAfter(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0
	}
	return seconds
}

// IsTimeout reports whether an error is a timeout, including net.Error and
// wrapped request deadlines.
func IsTimeout(err error) bool {
	var timeout *TimeoutError
	if errors.As(err, &timeout) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

// IsRedirectRefusal reports whether an error is a refused cross-origin redirect.
func IsRedirectRefusal(err error) bool {
	var redirect *RedirectError
	return errors.As(err, &redirect)
}
