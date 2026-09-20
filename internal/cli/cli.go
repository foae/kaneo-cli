// Package cli implements the kaneo-cli command-line interface.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/foae/kaneo-cli/internal/buildinfo"
	"github.com/spf13/cobra"
)

const commandName = "kaneo-cli"

type errorResponse struct {
	Error commandError `json:"error"`
}

type commandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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

// Execute runs the command with explicit process streams and returns its exit
// status. It does not call os.Exit, which keeps the command testable.
func Execute(args []string, in io.Reader, out, errOut io.Writer, info buildinfo.Info) int {
	root := NewRootCommand(info)
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)

	if err := root.Execute(); err != nil {
		writeError(errOut, err)
		if isUsageError(err) {
			return 2
		}
		return 1
	}

	return 0
}

// NewRootCommand creates the root command using the supplied build provenance.
func NewRootCommand(info buildinfo.Info) *cobra.Command {
	var showVersion bool

	root := &cobra.Command{
		Use:               commandName,
		Short:             "Command-line client for Kaneo",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Args:              noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				return writeVersion(cmd.OutOrStdout(), info)
			}
			if err := cmd.Help(); err != nil {
				return &processError{err: err}
			}
			return nil
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{err: err}
	})
	root.Flags().BoolVar(&showVersion, "version", false, "Print version information as JSON")
	root.AddCommand(newVersionCommand(info))

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
	if err := json.NewEncoder(out).Encode(info); err != nil {
		return &processError{err: err}
	}
	return nil
}

func writeError(out io.Writer, err error) {
	_ = json.NewEncoder(out).Encode(errorResponse{Error: commandError{
		Code:    errorCode(err),
		Message: err.Error(),
	}})
}

func isUsageError(err error) bool {
	var usage *usageError
	if errors.As(err, &usage) {
		return true
	}
	return strings.HasPrefix(err.Error(), "unknown command ")
}

func errorCode(err error) string {
	if strings.HasPrefix(err.Error(), "unknown command ") {
		return "unknown_command"
	}
	if isUsageError(err) {
		return "invalid_arguments"
	}
	return "process_failure"
}
