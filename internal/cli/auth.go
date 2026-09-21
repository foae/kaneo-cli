package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foae/kaneo-cli/internal/auth"
	"github.com/foae/kaneo-cli/internal/client"
	"github.com/foae/kaneo-cli/internal/config"
)

const maxAPIKeyBytes = 1 << 20

type loginResult struct {
	Profile string `json:"profile"`
	APIURL  string `json:"api_url"`
	Method  string `json:"method"`
	Storage string `json:"storage"`
}

func (a *app) newAuthCommand() *cobra.Command {
	group := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate and inspect the current session",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return help(cmd)
		},
	}
	group.AddCommand(a.newAuthLoginCommand(), a.newAuthLogoutCommand(), a.newAuthGetSessionCommand())
	return group
}

func (a *app) newAuthLoginCommand() *cobra.Command {
	var apiKeyFile string
	var clientID string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store an API key or sign in with the device flow",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			// Logging in to a named profile is how that profile is created; an
			// explicit profile that does not exist yet must not block login.
			if err := a.ensureLoginProfile(ctx); err != nil {
				return err
			}
			sess, err := a.session(ctx)
			if err != nil {
				return err
			}

			method := "device"
			secret := ""
			if apiKeyFile != "" {
				method = "api_key"
				secret, err = readAPIKey(a.streams.in, apiKeyFile)
				if err != nil {
					return &usageError{err: err}
				}
			} else {
				deviceClient, err := sess.newUnauthenticatedClient()
				if err != nil {
					return err
				}
				if clientID == "" {
					clientID = auth.DefaultClientID
				}
				token, err := a.deviceLogin(ctx, auth.DeviceOptions{
					Client:       deviceClient,
					ClientID:     clientID,
					Instructions: a.streams.errOut,
				})
				if err != nil {
					return err
				}
				secret = token.AccessToken
			}

			storage, err := sess.creds.Save(ctx, sess.resolved.ProfileName, sess.resolved.APIURL, secret)
			if err != nil {
				return &processError{err: err}
			}
			return writeJSONValue(cmd.OutOrStdout(), loginResult{
				Profile: sess.resolved.ProfileName,
				APIURL:  sess.resolved.APIURL,
				Method:  method,
				Storage: storage,
			})
		},
	}
	cmd.Flags().StringVar(&apiKeyFile, "api-key-file", "", "Read an API key from this file, or - for stdin")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Device authorization client ID (default kaneo-cli)")
	return cmd
}

// ensureLoginProfile creates an explicitly requested profile before login so a
// first login to a new named profile works without a separate `profile set`.
func (a *app) ensureLoginProfile(ctx context.Context) error {
	name := a.flags.profile
	if name == "" {
		name = a.env("KANEO_PROFILE")
	}
	if name == "" {
		return nil
	}
	dir, err := a.configDir()
	if err != nil {
		return &processError{err: err}
	}
	store := config.NewStore(dir)
	cfg, err := store.Load(ctx)
	if err != nil {
		return &processError{err: err}
	}
	if _, ok := cfg.Profile(name); ok {
		return nil
	}
	if err := store.Update(ctx, func(cfg *config.Config) error {
		profile := cfg.EnsureProfile(name)
		cfg.PutProfile(name, *profile)
		if cfg.DefaultProfile == "" {
			cfg.DefaultProfile = name
		}
		return nil
	}); err != nil {
		return &processError{err: err}
	}
	return nil
}

func (a *app) newAuthLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout [NAME]",
		Short: "Remove stored credentials for a profile",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if !a.flags.yes {
				return &usageError{err: errors.New("refusing to remove stored credentials without --yes")}
			}
			sess, err := a.session(ctx)
			if err != nil {
				return err
			}
			name, err := profileName(args, sess.resolved.ProfileName)
			if err != nil {
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
			return writeJSONValue(cmd.OutOrStdout(), map[string]any{"profile": name, "logged_out": true})
		},
	}
}

func (a *app) newAuthGetSessionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get-session",
		Short: "Get the current authenticated session, or null",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			sess, err := a.session(ctx)
			if err != nil {
				return err
			}
			apiClient, err := sess.newClient(ctx)
			if err != nil {
				return err
			}
			resp, err := apiClient.Do(ctx, client.Request{
				Method:      "GET",
				Path:        "/auth/get-session",
				OperationID: "getSession",
			})
			if err != nil {
				return err
			}
			return writeJSONStream(cmd.OutOrStdout(), resp)
		},
	}
}

// readAPIKey reads a secret from stdin or a protected file. It never accepts a
// secret as an argument and refuses a file readable by other users on Unix.
func readAPIKey(stdin io.Reader, path string) (string, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(io.LimitReader(stdin, maxAPIKeyBytes+1))
		if err != nil {
			return "", fmt.Errorf("read API key from stdin: %w", err)
		}
		if len(data) > maxAPIKeyBytes {
			return "", errors.New("API key input is too large")
		}
	} else {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return "", fmt.Errorf("read API key file: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("API key file %s must not be a symlink", path)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("API key file %s must not be accessible by group or others; use chmod 600", path)
		}
		handle, openErr := os.Open(path)
		if openErr != nil {
			return "", fmt.Errorf("read API key file: %w", openErr)
		}
		defer func() { _ = handle.Close() }()
		data, err = io.ReadAll(io.LimitReader(handle, maxAPIKeyBytes+1))
		if err != nil {
			return "", fmt.Errorf("read API key file: %w", err)
		}
		if len(data) > maxAPIKeyBytes {
			return "", errors.New("API key file is too large")
		}
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", errors.New("API key input was empty")
	}
	return secret, nil
}
