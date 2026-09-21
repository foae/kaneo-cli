// Command registry checks the release's fixed GHCR image without mutating it.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	repository  = "foae/kaneo-cli"
	registryURL = "https://ghcr.io"
	mediaTypes  = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
)

var (
	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	tagPattern    = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
)

type descriptor struct {
	Digest   string `json:"digest"`
	Platform struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	} `json:"platform"`
}

type manifest struct {
	Manifests []descriptor `json:"manifests"`
	Config    descriptor   `json:"config"`
}

type imageConfig struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Config       struct {
		User       string            `json:"User"`
		Entrypoint []string          `json:"Entrypoint"`
		Labels     map[string]string `json:"Labels"`
	} `json:"config"`
}

type registry struct {
	client *http.Client
	token  string
}

func registryRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 || req.URL.Scheme != "https" {
		return errors.New("unsafe registry redirect")
	}
	// Blob storage redirects may use signed URLs. Never forward registry auth.
	req.Header.Del("Authorization")
	return nil
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	r := registry{client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: registryRedirect}}
	return r.inspect(ctx, args)
}

func (r *registry) inspect(ctx context.Context, args []string) error {
	if len(args) < 2 || !tagPattern.MatchString(args[1]) {
		return errors.New("usage: registry absent vX.Y.Z | registry verify vX.Y.Z commit [expected-digest]")
	}
	mode, tag := args[0], args[1]
	if (mode != "absent" || len(args) != 2) && (mode != "verify" || len(args) < 3 || len(args) > 4) {
		return errors.New("invalid registry inspection arguments")
	}
	if err := r.authenticate(ctx, mode == "absent"); err != nil {
		return err
	}
	data, status, err := r.get(ctx, "/v2/"+repository+"/manifests/"+tag)
	if err != nil {
		return err
	}
	if mode == "absent" {
		if status == http.StatusNotFound {
			return nil
		}
		if status == http.StatusOK {
			return errors.New("versioned container tag already exists; refusing publication")
		}
		return fmt.Errorf("cannot establish registry absence: HTTP %d", status)
	}
	if status != http.StatusOK {
		return fmt.Errorf("cannot verify container: HTTP %d", status)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	if len(args) == 4 && (!digestPattern.MatchString(args[3]) || digest != args[3]) {
		return errors.New("container digest differs from release record")
	}
	if err := r.verify(ctx, data, tag, args[2]); err != nil {
		return err
	}
	fmt.Println(digest)
	return nil
}

func (r *registry) authenticate(ctx context.Context, creating bool) error {
	scope := "pull"
	// Creation checks need access to an as-yet nonexistent private package.
	credential := os.Getenv("GITHUB_TOKEN")
	if creating && credential != "" {
		scope = "pull,push"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL+"/token?service=ghcr.io&scope=repository:"+repository+":"+scope, nil)
	if err != nil {
		return errors.New("construct registry token request")
	}
	if credential != "" {
		actor := os.Getenv("GITHUB_ACTOR")
		if actor == "" {
			return errors.New("GITHUB_ACTOR is required with GITHUB_TOKEN")
		}
		req.SetBasicAuth(actor, credential)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return errors.New("registry authentication transport failure")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry authentication failed: HTTP %d (absence is not established)", resp.StatusCode)
	}
	var token struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil || token.Token == "" {
		return errors.New("invalid registry authentication response")
	}
	r.token = token.Token
	return nil
}

func (r *registry) get(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL+path, nil)
	if err != nil {
		return nil, 0, errors.New("construct registry request")
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", mediaTypes)
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, 0, errors.New("registry inspection transport failure")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, 0, errors.New("invalid or oversized registry response")
	}
	return data, resp.StatusCode, nil
}

func (r *registry) object(ctx context.Context, kind, digest string, target any) error {
	if !digestPattern.MatchString(digest) {
		return errors.New("invalid registry object digest")
	}
	data, status, err := r.get(ctx, "/v2/"+repository+"/"+kind+"/"+digest)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("registry object unavailable: HTTP %d", status)
	}
	if fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != digest {
		return errors.New("registry object digest mismatch")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errors.New("invalid registry object JSON")
	}
	return nil
}

func (r *registry) verify(ctx context.Context, data []byte, tag, commit string) error {
	var index manifest
	if err := json.Unmarshal(data, &index); err != nil {
		return errors.New("invalid container index")
	}
	seen := make(map[string]bool, 2)
	for _, entry := range index.Manifests {
		// BuildKit can append attestation descriptors with unknown platform.
		if entry.Platform.OS == "unknown" && entry.Platform.Architecture == "unknown" {
			continue
		}
		arch := entry.Platform.Architecture
		if entry.Platform.OS != "linux" || (arch != "amd64" && arch != "arm64") || seen[arch] {
			return errors.New("unexpected or duplicate container platform")
		}
		seen[arch] = true
		var image manifest
		if err := r.object(ctx, "manifests", entry.Digest, &image); err != nil {
			return err
		}
		var config imageConfig
		if err := r.object(ctx, "blobs", image.Config.Digest, &config); err != nil {
			return err
		}
		if config.OS != "linux" || config.Architecture != arch || config.Config.User != "65532:65532" || len(config.Config.Entrypoint) != 1 || config.Config.Entrypoint[0] != "/usr/local/bin/kaneo-cli" {
			return errors.New("container runtime contract mismatch")
		}
		for key, value := range map[string]string{"source": "https://github.com/" + repository, "revision": commit, "version": strings.TrimPrefix(tag, "v"), "licenses": "MIT"} {
			if config.Config.Labels["org.opencontainers.image."+key] != value {
				return fmt.Errorf("container %s label mismatch", key)
			}
		}
	}
	if len(seen) != 2 {
		return errors.New("container index must contain linux/amd64 and linux/arm64")
	}
	return nil
}
