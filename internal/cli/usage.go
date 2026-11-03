package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func usageFlagError(cmd *cobra.Command, err error) error {
	var unknown *pflag.NotExistError
	if !errors.As(err, &unknown) {
		return &usageError{err: err}
	}
	// pflag includes the entire shorthand group in its error, which can
	// contain an attached value. Only report the unrecognized flag itself.
	if unknown.GetSpecifiedShortnames() != "" {
		err = fmt.Errorf("unknown shorthand flag: %q", unknown.GetSpecifiedName())
	}

	if replacement := identifierFlagSuggestion(cmd, unknown.GetSpecifiedName()); replacement != "" {
		err = fmt.Errorf("%w. Did you mean --%s?", err, replacement)
	}
	// pflag retains positional arguments parsed before the unknown flag. These
	// can identify a misspelled subcommand without inspecting any flag values.
	return usageWithRecovery(cmd, err, cmd.Flags().Args())
}

func usageWithRecovery(cmd *cobra.Command, err error, args []string) error {
	var hints strings.Builder
	if cmd.HasAvailableSubCommands() && len(args) > 0 && !cmd.DisableSuggestions {
		// Match Cobra's default distance; SuggestionsFor itself does not apply
		// the default used by Cobra's own unknown-command diagnostics.
		if cmd.SuggestionsMinimumDistance <= 0 {
			cmd.SuggestionsMinimumDistance = 2
		}
		if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
			hints.WriteString(". Did you mean")
			for i, suggestion := range suggestions {
				if i > 0 {
					hints.WriteString(" or")
				}
				fmt.Fprintf(&hints, " '%s %s'", cmd.CommandPath(), suggestion)
			}
			hints.WriteString("?")
		}
	}
	fmt.Fprintf(&hints, " Run '%s --help' for usage", cmd.CommandPath())
	return &usageError{err: fmt.Errorf("%w%s", err, hints.String())}
}

func identifierFlagSuggestion(cmd *cobra.Command, unknown string) string {
	var replacement string
	switch unknown {
	case "id":
		replacement = "task-id"
	case "task-id":
		replacement = "id"
	default:
		return ""
	}

	// Correct only this explicit pair, never reinterpret another identifier.
	// Commands that already accept both flags need no correction.
	if cmd.Flags().Lookup(unknown) != nil {
		return ""
	}
	candidate := cmd.Flags().Lookup(replacement)
	if candidate == nil || candidate.Hidden || candidate.Deprecated != "" {
		return ""
	}
	// Unrelated scope flags do not change which spelling this command accepts.
	return replacement
}
