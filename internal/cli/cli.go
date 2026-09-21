// Package cli implements the kaneo-cli command-line interface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foae/kaneo-cli/internal/auth"
	"github.com/foae/kaneo-cli/internal/buildinfo"
	"github.com/foae/kaneo-cli/internal/client"
	"github.com/foae/kaneo-cli/internal/config"
)

const commandName = "kaneo-cli"

type errorResponse struct {
	Error commandError `json:"error"`
}

type commandError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Status      int    `json:"status,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

type usageError struct {
	err error
}

func (e *usageError) Error() string {
	return e.err.Error()
}

type processError struct {
	err error
}

func (e *processError) Error() string {
	return e.err.Error()
}

func (e *processError) Unwrap() error {
	return e.err
}

type streams struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

// globalFlags holds the values of the persistent flags.
type globalFlags struct {
	profile string
	apiURL  string
	timeout string
	yes     bool
}

// app carries the injected dependencies and process streams for one invocation.
type app struct {
	info    buildinfo.Info
	streams streams
	flags   globalFlags

	configDir       func() (string, error)
	credentialStore func(*config.Store, io.Writer) *auth.Store
	deviceLogin     func(context.Context, auth.DeviceOptions) (auth.DeviceToken, error)
	getenv          func(string) string
}

func defaultApp(info buildinfo.Info, in io.Reader, out, errOut io.Writer) *app {
	return &app{
		info:            info,
		streams:         streams{in: in, out: out, errOut: errOut},
		configDir:       config.DefaultDir,
		credentialStore: auth.NewStore,
		deviceLogin:     auth.LoginDevice,
		getenv:          os.Getenv,
	}
}

// Execute runs the command with explicit process streams and returns its exit
// status. It does not call os.Exit, which keeps the command testable.
func Execute(args []string, in io.Reader, out, errOut io.Writer, info buildinfo.Info) int {
	return execute(defaultApp(info, in, out, errOut), args)
}

func execute(application *app, args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return executeContext(ctx, application, args)
}

func executeContext(ctx context.Context, application *app, args []string) int {
	// contextcheck cannot see that Cobra propagates this ctx to every RunE via
	// cmd.Context(); command construction deliberately takes no context.
	root := application.newRootCommand() //nolint:contextcheck // RunE reads cmd.Context() set by ExecuteContext.
	root.SetArgs(args)
	root.SetIn(application.streams.in)
	root.SetOut(application.streams.out)
	root.SetErr(application.streams.errOut)

	if err := root.ExecuteContext(ctx); err != nil {
		application.writeError(err)
		return exitCode(err)
	}
	return 0
}

// NewRootCommand creates the root command using the supplied build provenance.
func NewRootCommand(info buildinfo.Info) *cobra.Command {
	return defaultApp(info, os.Stdin, os.Stdout, os.Stderr).newRootCommand()
}

func (a *app) newRootCommand() *cobra.Command {
	var showVersion bool

	root := &cobra.Command{
		Use:               commandName,
		Short:             "Command-line client for Kaneo",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Args:              noArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.ValidateFlagGroups(); err != nil {
				return &usageError{err: err}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				return writeVersion(cmd.OutOrStdout(), a.info)
			}
			return help(cmd)
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{err: err}
	})
	root.PersistentFlags().StringVar(&a.flags.profile, "profile", "", "Named profile to use")
	root.PersistentFlags().StringVar(&a.flags.apiURL, "api-url", "", "Full API base URL, including the API path")
	root.PersistentFlags().StringVar(&a.flags.timeout, "timeout", "", "Request timeout (for example 30s)")
	root.PersistentFlags().BoolVar(&a.flags.yes, "yes", false, "Confirm destructive operations without prompting")
	root.Flags().BoolVar(&showVersion, "version", false, "Print version information as JSON")

	root.AddCommand(newVersionCommand(a.info))
	root.AddCommand(a.newProfileCommand())
	root.AddCommand(a.newAuthCommand())
	root.AddCommand(a.newInstanceCommand())
	root.AddCommand(a.newConfigCommand())
	root.AddCommand(a.newAPIGroups()...)

	return root
}

func newVersionCommand(info buildinfo.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information as JSON",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeVersion(cmd.OutOrStdout(), info)
		},
	}
}

func help(cmd *cobra.Command) error {
	if err := cmd.Help(); err != nil {
		return &processError{err: err}
	}
	return nil
}

// usageArgs converts a Cobra argument validator's error into a usage error so
// argument-count mistakes exit 2 like flag errors.
func usageArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return &usageError{err: err}
		}
		return nil
	}
}

func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if cmd.Parent() == nil {
		return &usageError{err: fmt.Errorf("unknown command %q for %q", args[0], cmd.Name())}
	}
	return &usageError{err: fmt.Errorf("unexpected argument %q", args[0])}
}

func writeVersion(out io.Writer, info buildinfo.Info) error {
	return writeJSONValue(out, info)
}

func (a *app) writeError(err error) {
	payload := errorResponse{Error: commandError{
		Code:    errorCode(err),
		Message: safeMessage(err),
	}}
	var apiErr *client.Error
	if errors.As(err, &apiErr) {
		payload.Error.Status = apiErr.StatusCode
		payload.Error.OperationID = apiErr.OperationID
	}
	encoder := json.NewEncoder(a.streams.errOut)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

// safeMessage avoids rendering an underlying error that may embed a
// secret-bearing URL for transport and timeout failures.
func safeMessage(err error) string {
	var transport *client.TransportError
	if errors.As(err, &transport) {
		return transport.Error()
	}
	var timeout *client.TimeoutError
	if errors.As(err, &timeout) {
		return timeout.Error()
	}
	return err.Error()
}

func isUsageError(err error) bool {
	var usage *usageError
	if errors.As(err, &usage) {
		return true
	}
	// Never classify a typed API or transport failure as a usage error even if
	// its message happens to start with the cobra wording.
	var apiErr *client.Error
	if errors.As(err, &apiErr) {
		return false
	}
	var timeout *client.TimeoutError
	if errors.As(err, &timeout) {
		return false
	}
	var transport *client.TransportError
	if errors.As(err, &transport) {
		return false
	}
	return strings.HasPrefix(err.Error(), "unknown command ")
}

func errorCode(err error) string {
	switch {
	case strings.HasPrefix(err.Error(), "unknown command "):
		return "unknown_command"
	case isUsageError(err):
		return "invalid_arguments"
	case errors.Is(err, auth.ErrDeviceAccessDenied):
		return "access_denied"
	case errors.Is(err, auth.ErrDeviceExpired):
		return "expired_token"
	case errors.Is(err, auth.ErrDeviceInvalidClient):
		return "invalid_client"
	case errors.Is(err, auth.ErrCredentialMissing):
		return "credential_missing"
	case errors.Is(err, context.Canceled):
		return "interrupted"
	}
	var timeout *client.TimeoutError
	if errors.As(err, &timeout) {
		return "timeout"
	}
	var transport *client.TransportError
	if errors.As(err, &transport) {
		return "transport_failure"
	}
	var redirect *client.RedirectError
	if errors.As(err, &redirect) {
		return "cross_origin_redirect_refused"
	}
	var apiErr *client.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return "process_failure"
}

func exitCode(err error) int {
	switch {
	case isUsageError(err):
		return 2
	case errors.Is(err, auth.ErrDeviceAccessDenied),
		errors.Is(err, auth.ErrDeviceExpired),
		errors.Is(err, auth.ErrDeviceInvalidClient),
		errors.Is(err, auth.ErrCredentialMissing):
		return 3
	case errors.Is(err, context.Canceled):
		return 130
	}
	var timeout *client.TimeoutError
	if errors.As(err, &timeout) || errors.Is(err, context.DeadlineExceeded) {
		return 4
	}
	var transport *client.TransportError
	if errors.As(err, &transport) {
		return 4
	}
	var redirect *client.RedirectError
	if errors.As(err, &redirect) {
		return 4
	}
	var apiErr *client.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden {
			return 3
		}
		return 5
	}
	return 1
}
