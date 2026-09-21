package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/foae/kaneo-cli/internal/client"
)

// DefaultClientID is the documented default device client ID for this CLI.
const DefaultClientID = "kaneo-cli"

const (
	deviceCodePath  = "/auth/device/code"
	deviceTokenPath = "/auth/device/token"
	deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	defaultInterval = 5 * time.Second
	defaultExpiry   = 10 * time.Minute
	maxDeviceBody   = 1 << 20
	// maxPollInterval bounds a server-requested interval so a hostile value
	// cannot stall the flow.
	maxPollInterval = 10 * time.Minute
)

// ErrDeviceAccessDenied reports a denied authorization request.
var ErrDeviceAccessDenied = errors.New("device authorization was denied")

// ErrDeviceExpired reports an expired device code or exhausted wait.
var ErrDeviceExpired = errors.New("device authorization expired")

// ErrDeviceInvalidClient reports a client ID the server does not allow.
var ErrDeviceInvalidClient = errors.New("device client id is not allowed by this server")

// DeviceCode is the documented code-request response.
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

// DeviceToken is the documented successful polling response.
type DeviceToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// DeviceOptions configures a device login.
type DeviceOptions struct {
	Client       *client.Client
	ClientID     string
	Instructions io.Writer
	// Sleep and Now are injectable for deterministic tests.
	Sleep func(ctx context.Context, d time.Duration) error
	Now   func() time.Time
}

// LoginDevice runs the RFC 8628 device flow: request a code, show safe
// instructions, then poll until approved, denied or expired. It returns the
// access token only after documented success and never prints the device code
// or token.
func LoginDevice(ctx context.Context, opts DeviceOptions) (DeviceToken, error) {
	if opts.Client == nil {
		return DeviceToken{}, errors.New("device login requires an HTTP client")
	}
	clientID := opts.ClientID
	if clientID == "" {
		clientID = DefaultClientID
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	code, err := requestDeviceCode(ctx, opts.Client, clientID)
	if err != nil {
		return DeviceToken{}, err
	}
	verificationURI, err := safeVerificationURL(code.VerificationURI)
	if err != nil {
		return DeviceToken{}, err
	}
	completeURI, _ := safeVerificationURL(code.VerificationURIComplete)
	writeInstructions(opts.Instructions, verificationURI, completeURI, code.UserCode)

	interval := defaultInterval
	if code.Interval > 0 {
		interval = secondsDuration(code.Interval)
	}
	expiry := defaultExpiry
	if code.ExpiresIn > 0 {
		expiry = secondsDuration(code.ExpiresIn)
	}
	deadline := now().Add(expiry)

	for {
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return DeviceToken{}, ErrDeviceExpired
		}
		// Never sleep past the deadline, so an interval larger than the
		// remaining lifetime still expires on time.
		wait := interval
		if wait > remaining {
			wait = remaining
		}
		if err := sleep(ctx, wait); err != nil {
			return DeviceToken{}, err
		}
		if !now().Before(deadline) {
			return DeviceToken{}, ErrDeviceExpired
		}

		token, err := pollDeviceToken(ctx, opts.Client, clientID, code.DeviceCode)
		if err == nil {
			return token, nil
		}
		var apiErr *client.Error
		if !errors.As(err, &apiErr) {
			return DeviceToken{}, err
		}
		switch apiErr.ServerCode {
		case "authorization_pending":
			continue
		case "slow_down":
			if interval < maxPollInterval {
				interval += 5 * time.Second
			}
			continue
		case "access_denied":
			return DeviceToken{}, ErrDeviceAccessDenied
		case "expired_token":
			return DeviceToken{}, ErrDeviceExpired
		case "invalid_client":
			return DeviceToken{}, ErrDeviceInvalidClient
		default:
			return DeviceToken{}, err
		}
	}
}

func requestDeviceCode(ctx context.Context, c *client.Client, clientID string) (DeviceCode, error) {
	body, err := json.Marshal(map[string]string{"client_id": clientID})
	if err != nil {
		return DeviceCode{}, err
	}
	resp, err := c.Do(ctx, client.Request{
		Method:      "POST",
		Path:        deviceCodePath,
		Body:        body,
		ContentType: "application/json",
		OperationID: "deviceCodeRequest",
		Sensitive:   true,
	})
	if err != nil {
		return DeviceCode{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var code DeviceCode
	if err := decodeBounded(resp.Body, &code); err != nil {
		return DeviceCode{}, fmt.Errorf("decode device code response: %w", err)
	}
	if code.DeviceCode == "" || code.UserCode == "" || code.VerificationURI == "" {
		return DeviceCode{}, errors.New("device code response is missing required fields")
	}
	return code, nil
}

func pollDeviceToken(ctx context.Context, c *client.Client, clientID, deviceCode string) (DeviceToken, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type":  deviceGrantType,
		"device_code": deviceCode,
		"client_id":   clientID,
	})
	if err != nil {
		return DeviceToken{}, err
	}
	resp, err := c.Do(ctx, client.Request{
		Method:      "POST",
		Path:        deviceTokenPath,
		Body:        body,
		ContentType: "application/json",
		OperationID: "deviceTokenPoll",
		Sensitive:   true,
	})
	if err != nil {
		return DeviceToken{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
	}
	if err := decodeBounded(resp.Body, &payload); err != nil {
		return DeviceToken{}, fmt.Errorf("decode device token response: %w", err)
	}
	if payload.AccessToken == "" {
		// Some servers return a documented polling status with a 2xx status;
		// surface it as a retryable/terminal protocol error rather than a hard
		// decode failure.
		if payload.Error != "" {
			return DeviceToken{}, &client.Error{
				StatusCode: 0,
				Code:       "device_error",
				Message:    payload.Error,
				ServerCode: payload.Error,
			}
		}
		return DeviceToken{}, errors.New("device token response is missing an access token")
	}
	if payload.TokenType != "" && !strings.EqualFold(payload.TokenType, "Bearer") {
		return DeviceToken{}, fmt.Errorf("unsupported device token type %q", SanitizeForDisplay(payload.TokenType))
	}
	return DeviceToken{AccessToken: payload.AccessToken, TokenType: payload.TokenType}, nil
}

func secondsDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	const maxSeconds = int64(^uint64(0)>>1) / int64(time.Second)
	if int64(seconds) > maxSeconds {
		return time.Duration(maxSeconds) * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func decodeBounded(reader io.Reader, target any) error {
	payload, err := io.ReadAll(io.LimitReader(reader, maxDeviceBody+1))
	if err != nil {
		return err
	}
	if len(payload) > maxDeviceBody {
		return fmt.Errorf("device response exceeds %d bytes", maxDeviceBody)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return client.ErrInvalidJSONResponse
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// safeVerificationURL accepts only absolute http/https URLs. The empty string
// maps to the empty string so the optional complete URI can be absent.
func safeVerificationURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("server returned an invalid device verification URL")
	}
	if parsed.User != nil {
		return "", errors.New("server returned a device verification URL with user information")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("server returned a device verification URL with unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("server returned a device verification URL without a host")
	}
	return parsed.String(), nil
}

func writeInstructions(out io.Writer, verificationURI, completeURI, userCode string) {
	if out == nil {
		return
	}
	if completeURI != "" {
		_, _ = fmt.Fprintf(out, "To sign in, open %s\n", SanitizeForDisplay(completeURI))
	} else {
		_, _ = fmt.Fprintf(out, "To sign in, open %s\n", SanitizeForDisplay(verificationURI))
	}
	if userCode != "" {
		_, _ = fmt.Fprintf(out, "Then enter the code %s\n", SanitizeForDisplay(userCode))
	}
	_, _ = fmt.Fprintln(out, "Waiting for approval... (press Ctrl+C to cancel)")
}

// SanitizeForDisplay strips control characters and Unicode bidirectional
// overrides from server-provided text before it reaches a terminal.
func SanitizeForDisplay(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 || r == 0x7f:
			return -1
		case r == 0x200e || r == 0x200f:
			return -1
		case r >= 0x202a && r <= 0x202e:
			return -1
		case r >= 0x2066 && r <= 0x2069:
			return -1
		default:
			return r
		}
	}, value)
}
