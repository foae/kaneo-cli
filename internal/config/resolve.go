package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/foae/kaneo-cli/internal/client"
)

// Documented defaults. The API URL is the pinned OpenAPI server; the timeout
// and profile name are this CLI's chosen defaults and are documented in
// docs/authentication.md.
const (
	DefaultAPIURL      = "https://cloud.kaneo.app/api"
	DefaultTimeout     = 30 * time.Second
	DefaultProfileName = "default"
)

// Resolver resolves one effective configuration from flag, environment and
// profile inputs. Empty strings mean "not supplied".
type Resolver struct {
	Config *Config

	FlagProfile string
	EnvProfile  string
	FlagAPIURL  string
	EnvAPIURL   string
	FlagTimeout string
	EnvTimeout  string
}

// Resolved is the effective configuration for one invocation. Sources records
// where each nonsecret setting came from without revealing secrets.
type Resolved struct {
	ProfileName   string
	Profile       Profile
	ProfileExists bool
	APIURL        string
	Base          client.BaseURL
	Timeout       time.Duration
	Sources       map[string]string
}

// ResolveProfile applies profile selection without resolving request settings.
// Local configuration and credential management do not need an API destination.
func (r Resolver) ResolveProfile() (Resolved, error) {
	result := Resolved{Sources: map[string]string{}}

	switch {
	case r.FlagProfile != "":
		result.ProfileName = r.FlagProfile
		result.Sources["profile"] = "flag"
	case r.EnvProfile != "":
		result.ProfileName = r.EnvProfile
		result.Sources["profile"] = "environment"
	case r.Config != nil && r.Config.DefaultProfile != "":
		result.ProfileName = r.Config.DefaultProfile
		result.Sources["profile"] = "default_profile"
	default:
		result.ProfileName = DefaultProfileName
		result.Sources["profile"] = "default"
	}

	if r.Config != nil {
		if profile, ok := r.Config.Profile(result.ProfileName); ok {
			result.Profile = profile
			result.ProfileExists = true
		} else if r.FlagProfile != "" || r.EnvProfile != "" {
			// An explicitly requested profile must exist; a typo must not
			// silently fall back to another URL.
			return Resolved{}, fmt.Errorf("profile %q does not exist", result.ProfileName)
		}
	}
	return result, nil
}

// Resolve applies documented precedence: flags, then environment, then the
// selected profile, then defaults.
func (r Resolver) Resolve() (Resolved, error) {
	result, err := r.ResolveProfile()
	if err != nil {
		return Resolved{}, err
	}

	apiURL := r.FlagAPIURL
	switch {
	case r.FlagAPIURL != "":
		result.Sources["api_url"] = "flag"
	case r.EnvAPIURL != "":
		apiURL = r.EnvAPIURL
		result.Sources["api_url"] = "environment"
	case result.Profile.APIURL != "":
		apiURL = result.Profile.APIURL
		result.Sources["api_url"] = "profile"
	default:
		apiURL = DefaultAPIURL
		result.Sources["api_url"] = "default"
	}
	normalized, err := client.ParseBaseURL(apiURL)
	if err != nil {
		return Resolved{}, fmt.Errorf("invalid API URL: %w", err)
	}
	result.APIURL = normalized.String()
	result.Base = normalized

	timeout := r.FlagTimeout
	switch {
	case r.FlagTimeout != "":
		result.Sources["timeout"] = "flag"
	case r.EnvTimeout != "":
		timeout = r.EnvTimeout
		result.Sources["timeout"] = "environment"
	case result.Profile.Timeout != "":
		timeout = result.Profile.Timeout
		result.Sources["timeout"] = "profile"
	default:
		result.Timeout = DefaultTimeout
		result.Sources["timeout"] = "default"
	}
	if timeout != "" {
		parsed, err := time.ParseDuration(timeout)
		if err != nil {
			return Resolved{}, fmt.Errorf("invalid timeout %q: %w", timeout, err)
		}
		if parsed <= 0 {
			return Resolved{}, errors.New("timeout must be greater than zero")
		}
		result.Timeout = parsed
	}

	return result, nil
}
