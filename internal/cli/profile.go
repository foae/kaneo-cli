package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/foae/kaneo-cli/internal/config"
)

type profileView struct {
	Name          string            `json:"name"`
	APIURL        string            `json:"api_url,omitempty"`
	Timeout       string            `json:"timeout,omitempty"`
	Default       bool              `json:"default"`
	Authenticated bool              `json:"authenticated"`
	Storage       string            `json:"storage,omitempty"`
	Sources       map[string]string `json:"sources,omitempty"`
}

func (a *app) newProfileCommand() *cobra.Command {
	group := &cobra.Command{
		Use:   "profile",
		Short: "Manage local profiles and their nonsecret settings",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return help(cmd)
		},
	}
	group.AddCommand(a.newProfileSetCommand(), a.newProfileUseCommand(), a.newProfileListCommand(), a.newProfileGetCommand(), a.newProfileDeleteCommand())
	return group
}

// localSession resolves only profile selection for local commands. It
// deliberately skips API URL and timeout resolution so malformed inherited
// request settings cannot block local profile or credential management.
func (a *app) localSession(ctx context.Context) (*session, error) {
	dir, err := a.configDir()
	if err != nil {
		return nil, err
	}
	store := config.NewStore(dir)
	cfg, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}

	resolved, err := a.resolver(cfg).ResolveProfile()
	if err != nil {
		return nil, &usageError{err: err}
	}

	return &session{
		ctx:      ctx,
		app:      a,
		store:    store,
		cfg:      cfg,
		resolved: resolved,
		creds:    a.credentialStore(store, a.streams.errOut),
	}, nil
}

func (a *app) newProfileSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set [NAME]",
		Short: "Create or update a profile's API URL and timeout",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			sess, err := a.localSession(ctx)
			if err != nil {
				return err
			}
			name, err := profileName(args, sess.resolved.ProfileName)
			if err != nil {
				return err
			}
			if a.flags.apiURL == "" && a.flags.timeout == "" {
				return &usageError{err: errors.New("nothing to set: supply --api-url or --timeout")}
			}
			normalizedURL := ""
			if a.flags.apiURL != "" {
				normalizedURL, err = config.URLKey(a.flags.apiURL)
				if err != nil {
					return &usageError{err: fmt.Errorf("invalid --api-url: %w", err)}
				}
			}
			if a.flags.timeout != "" {
				if err := validateTimeout(a.flags.timeout); err != nil {
					return err
				}
			}
			var view profileView
			err = sess.store.Update(ctx, func(cfg *config.Config) error {
				profile := cfg.EnsureProfile(name)
				if normalizedURL != "" {
					profile.APIURL = normalizedURL
				}
				if a.flags.timeout != "" {
					profile.Timeout = a.flags.timeout
				}
				effectiveURL := normalizedURL
				if effectiveURL == "" {
					effectiveURL = profile.APIURL
				}
				cfg.PutProfile(name, *profile)
				if cfg.DefaultProfile == "" {
					cfg.DefaultProfile = name
				}
				view = buildProfileView(cfg, name, effectiveURL, false, nil)
				return nil
			})
			if err != nil {
				return &processError{err: err}
			}
			return writeJSONValue(cmd.OutOrStdout(), view)
		},
	}
}

func (a *app) newProfileUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use NAME",
		Short: "Select the default profile",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			sess, err := a.localSession(ctx)
			if err != nil {
				return err
			}
			name := args[0]
			if err := validateProfileName(name); err != nil {
				return err
			}
			err = sess.store.Update(ctx, func(cfg *config.Config) error {
				if _, ok := cfg.Profile(name); !ok {
					return &usageError{err: fmt.Errorf("profile %q does not exist", name)}
				}
				cfg.DefaultProfile = name
				return nil
			})
			if err != nil {
				return err
			}
			return writeJSONValue(cmd.OutOrStdout(), map[string]string{"default_profile": name})
		},
	}
}

func (a *app) newProfileListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local profiles",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			sess, err := a.localSession(ctx)
			if err != nil {
				return err
			}
			views := make([]profileView, 0, len(sess.cfg.Profiles))
			for _, name := range sess.cfg.Names() {
				profile, _ := sess.cfg.Profile(name)
				views = append(views, buildProfileView(sess.cfg, name, profile.APIURL, false, nil))
			}
			return writeJSONValue(cmd.OutOrStdout(), views)
		},
	}
}

func (a *app) newProfileGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get [NAME]",
		Short: "Show effective settings and credential backend for a profile",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := a.configDir()
			if err != nil {
				return err
			}
			cfg, err := config.NewStore(dir).Load(cmd.Context())
			if err != nil {
				return err
			}
			resolver := a.resolver(cfg)
			if len(args) != 0 {
				name, err := profileName(args, "")
				if err != nil {
					return err
				}
				resolver.FlagProfile = name
			}
			resolved, err := resolver.Resolve()
			if err != nil {
				return &usageError{err: err}
			}
			if len(args) != 0 {
				resolved.Sources["profile"] = "argument"
			}
			view := buildProfileView(cfg, resolved.ProfileName, resolved.APIURL, true, resolved.Sources)
			view.APIURL = resolved.APIURL
			view.Timeout = resolved.Timeout.String()
			return writeJSONValue(cmd.OutOrStdout(), view)
		},
	}
}

func (a *app) newProfileDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a local profile and its stored credential",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if !a.flags.yes {
				return &usageError{err: errors.New("refusing to delete a profile without --yes")}
			}
			sess, err := a.localSession(ctx)
			if err != nil {
				return err
			}
			name := args[0]
			if err := validateProfileName(name); err != nil {
				return err
			}
			profile, ok := sess.cfg.Profile(name)
			if !ok {
				return &usageError{err: fmt.Errorf("profile %q does not exist", name)}
			}
			for _, apiURL := range profile.CredentialURLs() {
				if err := sess.creds.Delete(ctx, name, apiURL); err != nil {
					return &processError{err: err}
				}
			}
			if err := sess.store.Update(ctx, func(cfg *config.Config) error {
				cfg.DeleteProfile(name)
				return nil
			}); err != nil {
				return &processError{err: err}
			}
			return writeJSONValue(cmd.OutOrStdout(), map[string]any{"name": name, "deleted": true})
		},
	}
}

func buildProfileView(cfg *config.Config, name, effectiveURL string, includeSources bool, sources map[string]string) profileView {
	profile, ok := cfg.Profile(name)
	view := profileView{Name: name, Default: cfg.DefaultProfile == name}
	if ok {
		view.APIURL = profile.APIURL
		view.Timeout = profile.Timeout
		if effectiveURL != "" {
			if ref, found := profile.CredentialFor(effectiveURL); found {
				view.Authenticated = true
				view.Storage = ref.Backend
			}
		}
	}
	if includeSources && len(sources) > 0 {
		view.Sources = sources
	}
	return view
}

func profileName(args []string, fallback string) (string, error) {
	if len(args) == 0 {
		if err := validateProfileName(fallback); err != nil {
			return "", err
		}
		return fallback, nil
	}
	if err := validateProfileName(args[0]); err != nil {
		return "", err
	}
	return args[0], nil
}

func validateProfileName(name string) error {
	if name == "" {
		return &usageError{err: errors.New("profile name is empty")}
	}
	if name == "." || name == ".." {
		return &usageError{err: fmt.Errorf("profile name %q is not allowed", name)}
	}
	if strings.ContainsAny(name, `/\`) {
		return &usageError{err: fmt.Errorf("profile name %q must not contain path separators", name)}
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return &usageError{err: errors.New("profile name must not contain control characters")}
		}
	}
	return nil
}

func validateTimeout(value string) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return &usageError{err: fmt.Errorf("invalid timeout %q: %w", value, err)}
	}
	if duration <= 0 {
		return &usageError{err: errors.New("timeout must be greater than zero")}
	}
	return nil
}
