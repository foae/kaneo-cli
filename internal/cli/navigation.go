package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

type navigationParam struct {
	readParam
	maxLen int
}

func (a *app) newAuthDeviceAuthorizationPageCommand() *cobra.Command {
	params := []navigationParam{
		{readParam: readParam{name: "user_code", in: paramQuery, flag: "user-code", help: "Device authorization user code"}},
		{readParam: readParam{name: "ui", in: paramQuery, flag: "ui", enum: []string{"1"}, help: "Force the web UI redirect"}},
	}
	cmd := &cobra.Command{
		Use:   "get-device-authorization-page",
		Short: "Print the device authorization page URL",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, err := navigationQuery(cmd, params)
			if err != nil {
				return err
			}
			// This command always hands off to the browser page rather than the
			// JSON device payload served to non-navigation requests.
			query.Set("ui", "1")
			return a.writeNavigationURL(cmd, "/auth/device", query)
		},
	}
	addNavigationFlags(cmd, params)
	return cmd
}

func (a *app) newMCPAuthorizationCommand() *cobra.Command {
	params := []navigationParam{
		{readParam: readParam{name: "response_type", in: paramQuery, flag: "response-type", required: true, enum: []string{"code"}, help: "OAuth response type"}},
		{readParam: readParam{name: "client_id", in: paramQuery, flag: "client-id", required: true, help: "OAuth client ID"}},
		{readParam: readParam{name: "redirect_uri", in: paramQuery, flag: "redirect-uri", required: true, help: "OAuth redirect URI"}, maxLen: 2048},
		{readParam: readParam{name: "code_challenge", in: paramQuery, flag: "code-challenge", required: true, minLen: 1, help: "S256 PKCE code challenge"}},
		{readParam: readParam{name: "code_challenge_method", in: paramQuery, flag: "code-challenge-method", required: true, enum: []string{"S256"}, help: "PKCE code challenge method"}},
		{readParam: readParam{name: "state", in: paramQuery, flag: "state", help: "OAuth state"}},
	}
	cmd := &cobra.Command{
		Use:   "start-authorization",
		Short: "Print the MCP authorization URL",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query, err := navigationQuery(cmd, params)
			if err != nil {
				return err
			}
			return a.writeNavigationURL(cmd, "/mcp/authorize", query)
		},
	}
	addNavigationFlags(cmd, params)
	return cmd
}

func (a *app) newNavigationCommands() []groupedCommand {
	return []groupedCommand{
		{group: "mcp", cmd: a.newMCPAuthorizationCommand()},
	}
}

func addNavigationFlags(cmd *cobra.Command, params []navigationParam) {
	for _, param := range params {
		help := param.help
		if len(param.enum) > 0 {
			help += " (one of: " + strings.Join(param.enum, ", ") + ")"
		}
		if param.required {
			help = strings.TrimSpace(help + " (required)")
		}
		cmd.Flags().String(param.flag, "", help)
	}
}

func navigationQuery(cmd *cobra.Command, params []navigationParam) (url.Values, error) {
	query := make(url.Values, len(params))
	for _, param := range params {
		flag := cmd.Flags().Lookup(param.flag)
		if flag == nil {
			return nil, &processError{err: fmt.Errorf("internal error: flag --%s is not registered", param.flag)}
		}
		if !flag.Changed {
			if param.required {
				return nil, &usageError{err: fmt.Errorf("--%s is required", param.flag)}
			}
			continue
		}
		value := flag.Value.String()
		if err := validateParam(param.readParam, value); err != nil {
			return nil, &usageError{err: err}
		}
		if param.maxLen > 0 && len(value) > param.maxLen {
			return nil, &usageError{err: fmt.Errorf("--%s must be at most %d characters", param.flag, param.maxLen)}
		}
		query.Set(param.name, value)
	}
	return query, nil
}

func (a *app) writeNavigationURL(cmd *cobra.Command, path string, query url.Values) error {
	sess, err := a.session(cmd.Context())
	if err != nil {
		return err
	}
	destination, err := sess.resolved.Base.Join(path)
	if err != nil {
		return &processError{err: err}
	}
	destination.RawQuery = query.Encode()
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), destination.String()); err != nil {
		return &processError{err: fmt.Errorf("write navigation URL: %w", err)}
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "This command only prints the authorization URL; open it in a browser to continue."); err != nil {
		return &processError{err: fmt.Errorf("write navigation guidance: %w", err)}
	}
	if sess.resolved.Base.Scheme() == "http" {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning: this URL uses HTTP; browser cookies may require HTTPS."); err != nil {
			return &processError{err: fmt.Errorf("write navigation warning: %w", err)}
		}
	}
	return nil
}
