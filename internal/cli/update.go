package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	latestReleaseAPI  = "https://api.github.com/repos/foae/kaneo-cli/releases/latest"
	latestReleasePage = "https://github.com/foae/kaneo-cli/releases/latest"
	updateTimeout     = time.Second
)

func newUpdateClient() *http.Client {
	return &http.Client{
		Timeout: updateTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// notifyUpdate is best-effort and separate from the credentialed Kaneo client.
// Only stable release builds participate; local and prerelease builds are not
// comparable to the stable release channel advertised here.
func (a *app) notifyUpdate(ctx context.Context, out io.Writer) {
	current, ok := parseStableVersion(a.info.Version)
	if !ok || a.updateClient == nil || ctx == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseAPI, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", commandName)
	resp, err := a.updateClient.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil || len(body) > 64*1024 {
		return
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.Unmarshal(body, &release) != nil || release.Draft || release.Prerelease {
		return
	}
	latest, ok := parseStableVersion(release.Tag)
	if !ok || !current.less(latest) {
		return
	}
	_, _ = fmt.Fprintf(out, "Update available: kaneo-cli %s -> %s; upgrade: %s\n", a.info.Version, release.Tag, latestReleasePage)
}

type stableVersion [3]string

// Keep numeric components as strings to compare without integer overflow.
func parseStableVersion(value string) (stableVersion, bool) {
	var version stableVersion
	value = strings.TrimPrefix(value, "v")
	for i := range version {
		part, rest, more := strings.Cut(value, ".")
		if more != (i < len(version)-1) || part == "" || (len(part) > 1 && part[0] == '0') {
			return stableVersion{}, false
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return stableVersion{}, false
			}
		}
		version[i] = part
		value = rest
	}
	return version, true
}

func (v stableVersion) less(other stableVersion) bool {
	for i := range v {
		if len(v[i]) != len(other[i]) {
			return len(v[i]) < len(other[i])
		}
		if v[i] != other[i] {
			return v[i] < other[i]
		}
	}
	return false
}
