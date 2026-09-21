package client

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// BaseURL is a validated, normalized API base URL. It includes the API path
// (for example https://cloud.kaneo.app/api) so command paths are joined onto it
// exactly once.
type BaseURL struct {
	url    *url.URL
	origin string
}

// ParseBaseURL validates a full API base URL and produces its normalized form.
// The scheme must be HTTP or HTTPS, userinfo is rejected, and the normalized
// value drops a default port and a trailing slash. An empty URL is invalid so
// callers must supply the documented default explicitly.
func ParseBaseURL(raw string) (BaseURL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return BaseURL{}, errors.New("API base URL is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return BaseURL{}, fmt.Errorf("API base URL is malformed: %w", err)
	}
	if parsed.User != nil {
		return BaseURL{}, errors.New("API base URL must not contain user information")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return BaseURL{}, fmt.Errorf("API base URL must use http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return BaseURL{}, errors.New("API base URL has no host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return BaseURL{}, errors.New("API base URL must not contain a query or fragment")
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	switch {
	case port != "":
		parsed.Host = net.JoinHostPort(hostname, port)
	case strings.Contains(hostname, ":"):
		// Restore brackets on an IPv6 literal so the normalized URL re-parses.
		parsed.Host = "[" + hostname + "]"
	default:
		parsed.Host = hostname
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	// Preserve percent-encoding: a base path containing %2F must keep its own
	// credential binding rather than collapsing to its decoded form.
	if parsed.RawPath != "" {
		parsed.RawPath = strings.TrimSuffix(parsed.RawPath, "/")
		if parsed.RawPath == "" {
			parsed.RawPath = "/"
		}
	}

	return BaseURL{url: parsed, origin: originOf(parsed)}, nil
}

// String returns the normalized base URL without a trailing slash (except for
// the root path).
func (b BaseURL) String() string {
	if b.url == nil {
		return ""
	}
	return b.url.String()
}

// Origin returns the normalized scheme://host[:port] of the base URL.
func (b BaseURL) Origin() string { return b.origin }

// Scheme returns the normalized scheme.
func (b BaseURL) Scheme() string {
	if b.url == nil {
		return ""
	}
	return b.url.Scheme
}

// Host returns the normalized host, including a non-default port.
func (b BaseURL) Host() string {
	if b.url == nil {
		return ""
	}
	return b.url.Host
}

// Join appends an API path to the base path. The path must not be empty and is
// joined without duplicating separators. It is treated as already
// percent-encoded so an escaped path-parameter value containing a separator
// cannot change the request target; the decoded form is kept alongside for the
// URL package to re-emit exactly.
func (b BaseURL) Join(apiPath string) (*url.URL, error) {
	if b.url == nil {
		return nil, errors.New("API base URL is not set")
	}
	if apiPath == "" {
		return nil, errors.New("API path is empty")
	}
	escaped := strings.TrimSuffix(b.url.EscapedPath(), "/") + "/" + strings.TrimPrefix(apiPath, "/")
	decoded, err := url.PathUnescape(escaped)
	if err != nil {
		return nil, fmt.Errorf("API path is not valid: %w", err)
	}
	joined := *b.url
	joined.Path = decoded
	joined.RawPath = escaped
	return &joined, nil
}
