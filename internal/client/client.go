// Package client implements the HTTP transport shared by CLI commands.
package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// Options configures a Client.
type Options struct {
	// BaseURL is the validated API base URL.
	BaseURL BaseURL
	// HTTPClient overrides the default client; useful for tests and for
	// presigned transfers that must not carry credentials.
	HTTPClient *http.Client
	// Token is the bearer credential. An empty token is an unauthenticated
	// client.
	Token string
	// Timeout bounds every request when HTTPClient is nil. A zero timeout
	// selects the documented default.
	Timeout time.Duration
	// WarnWriter receives the plain-HTTP credential warning. Nil disables it.
	WarnWriter io.Writer
	// UserAgent overrides the default User-Agent.
	UserAgent string
}

// Client performs authenticated, bounded HTTP requests against one base URL.
type Client struct {
	base      BaseURL
	http      *http.Client
	token     string
	warn      io.Writer
	warned    bool
	userAgent string
}

// New creates a Client. The caller owns credential injection; a Client never
// mutates shared state and is safe for sequential use within one invocation.
func New(opts Options) (*Client, error) {
	if opts.BaseURL.String() == "" {
		return nil, errors.New("client: base URL is required")
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = defaultTimeout
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	copyOf := *httpClient
	httpClient = &copyOf
	existing := httpClient.CheckRedirect
	if existing == nil {
		existing = func(_ *http.Request, _ []*http.Request) error { return nil }
	}
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.User != nil {
			return &RedirectError{From: originOf(via[0].URL), To: "URL with user information"}
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if len(via) > 0 && originOf(req.URL) != originOf(via[0].URL) {
			return &RedirectError{From: originOf(via[0].URL), To: originOf(req.URL)}
		}
		return existing(req, via)
	}

	return &Client{
		base:      opts.BaseURL,
		http:      httpClient,
		token:     opts.Token,
		warn:      opts.WarnWriter,
		userAgent: opts.UserAgent,
	}, nil
}

// BaseURL returns the client's normalized base URL.
func (c *Client) BaseURL() BaseURL { return c.base }

// Authenticated reports whether the client carries a credential.
func (c *Client) Authenticated() bool { return c.token != "" }

// Request describes one API call. Body is passed through untouched so request
// encodings preserve their exact bytes.
type Request struct {
	Method      string
	Path        string
	Query       url.Values
	Body        []byte
	ContentType string
	OperationID string
	// Sensitive marks a body that carries a credential even though the request
	// has no Authorization header, so it triggers the plain-HTTP warning.
	Sensitive bool
	// SecretBody marks an operation whose request body carries a credential.
	// Such a server may echo a submitted value back in an error, so plain-text
	// error bodies are not surfaced for these operations.
	SecretBody bool
}

// Response is a successful (2xx) response. The caller must close Body.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

// Empty reports whether the response carries no content, including 204 and an
// explicit zero content length.
func (r *Response) Empty() bool {
	return r.StatusCode == http.StatusNoContent || r.Header.Get("Content-Length") == "0"
}

// Do executes the request, returning a Response for 2xx statuses and a typed
// error otherwise. Redirects to another origin are refused.
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	target, err := c.base.Join(req.Path)
	if err != nil {
		return nil, err
	}
	if req.Query != nil {
		target.RawQuery = req.Query.Encode()
	}

	var body io.Reader
	if req.Body != nil {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, target.String(), body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	if req.ContentType != "" {
		httpReq.Header.Set("Content-Type", req.ContentType)
	}
	if c.userAgent != "" {
		httpReq.Header.Set("User-Agent", c.userAgent)
	}
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	c.warnPlainHTTP(req.Sensitive)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, translateTransportError(err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		apiErr := newError(resp, req.OperationID, req.SecretBody)
		_ = resp.Body.Close()
		return nil, apiErr
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body}, nil
}

func (c *Client) warnPlainHTTP(sensitive bool) {
	if c.warn == nil || c.warned || !strings.EqualFold(c.base.Scheme(), "http") {
		return
	}
	if c.token == "" && !sensitive {
		return
	}
	c.warned = true
	_, _ = fmt.Fprintf(c.warn, "warning: sending credentials over plain HTTP to %s\n", c.base.Origin())
}

// originOf normalizes a URL to scheme://host[:port], omitting default ports so
// same-origin redirects with an explicit default port are not misjudged.
func originOf(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	switch {
	case port != "":
		return scheme + "://" + net.JoinHostPort(host, port)
	case strings.Contains(host, ":"):
		return scheme + "://[" + host + "]"
	default:
		return scheme + "://" + host
	}
}

func translateTransportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	var redirect *RedirectError
	if errors.As(err, &redirect) {
		return redirect
	}
	if errors.Is(err, context.DeadlineExceeded) || IsTimeout(err) {
		return &TimeoutError{Err: err}
	}
	return &TransportError{Err: err}
}
