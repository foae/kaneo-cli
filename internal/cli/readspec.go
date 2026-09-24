package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foae/kaneo-cli/internal/client"
)

// paramLocation is where a documented parameter appears in the request.
type paramLocation int

const (
	paramPath paramLocation = iota
	paramQuery
)

var numericPattern = regexp.MustCompile(`^\d+$`)

// readParam is one documented path or query parameter. Every parameter is
// exposed as a named flag and its value is passed to the wire verbatim; only
// explicitly supplied flags are sent, preserving omitted/default semantics.
type readParam struct {
	name     string // wire name
	in       paramLocation
	flag     string
	required bool
	enum     []string
	pattern  string
	numeric  bool
	minLen   int
	maxLen   int
	help     string
	// alias is an optional hidden alternative flag name for the same
	// parameter. It is accepted but never shown in help, so the canonical
	// spec-derived name stays the documented one.
	alias string
}

// readSpec is one read-only operation mapped to a CLI command.
type readSpec struct {
	group       string
	action      string
	short       string
	operationID string
	path        string
	params      []readParam
	// public marks an operation whose documented security is empty, so it must
	// work without a credential.
	public bool
	// optionalAuth marks a public operation that returns more for the caller
	// when a credential is available (for example an asset that is only public
	// when its project is). The stored credential is attached when present but
	// its absence is not an error.
	optionalAuth bool
	// binary marks an operation whose 200 response is a non-JSON stream that
	// must be written to an explicit destination.
	binary bool
	// oneRequired lists flag names of which at least one must be supplied.
	oneRequired []string
	// mutuallyExclusive lists flag names that may not be combined.
	mutuallyExclusive []string
	// requiredTogether lists flag names that must all be supplied if any is.
	requiredTogether []string
	// keyResolution, when set, registers client-side-only flags that resolve a
	// display key such as "KAN-12" into a path parameter before the request.
	keyResolution *keyResolution
}

// keyResolution maps a human display key onto an opaque identifier parameter.
type keyResolution struct {
	// flag is the client-side key flag, e.g. "key".
	flag string
	// target is the spec param flag it populates, e.g. "id".
	target string
	// help describes the key flag. It is per-operation because the key's shape
	// is domain-specific; the scope flag's help is derived from it.
	help string
	// scopeHelp describes the --workspace-id flag the resolution needs.
	scopeHelp string
}

func (a *app) newReadCommand(spec readSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   spec.action,
		Short: spec.short,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runRead(cmd, spec)
		},
	}
	for _, param := range spec.params {
		help := param.help
		if len(param.enum) > 0 {
			help = strings.TrimSpace(help + " (one of: " + strings.Join(param.enum, ", ") + ")")
		}
		if param.required {
			// A key-resolution target is still a required wire parameter, but
			// the key flag can supply it, so the help must not claim otherwise.
			if spec.keyResolution != nil && param.flag == spec.keyResolution.target {
				help = strings.TrimSpace(help + " (required unless --" + spec.keyResolution.flag + " is given)")
			} else {
				help = strings.TrimSpace(help + " (required)")
			}
		}
		cmd.Flags().String(param.flag, "", help)
		if param.alias != "" {
			cmd.Flags().String(param.alias, "", help)
			_ = cmd.Flags().MarkHidden(param.alias)
			cmd.MarkFlagsMutuallyExclusive(param.flag, param.alias)
		}
	}
	if spec.keyResolution != nil {
		cmd.Flags().String(spec.keyResolution.flag, "", spec.keyResolution.help)
		cmd.Flags().String("workspace-id", "", spec.keyResolution.scopeHelp)
	}
	if len(spec.oneRequired) > 0 {
		cmd.MarkFlagsOneRequired(spec.oneRequired...)
	}
	if len(spec.mutuallyExclusive) > 0 {
		cmd.MarkFlagsMutuallyExclusive(spec.mutuallyExclusive...)
	}
	if len(spec.requiredTogether) > 0 {
		cmd.MarkFlagsRequiredTogether(spec.requiredTogether...)
	}
	if spec.binary {
		cmd.Flags().String("output", "", "Destination file path, or - for stdout (required)")
		cmd.Flags().Bool("force", false, "Overwrite an existing destination file")
	}
	return cmd
}

func (a *app) runRead(cmd *cobra.Command, spec readSpec) error {
	ctx := cmd.Context()

	if spec.binary {
		if err := validateBinaryOutput(cmd); err != nil {
			return err
		}
	}
	// The session is built at most once per invocation, lazily, so resolution
	// and the final request always share one effective configuration and
	// commands that fail earlier never touch the credential backend.
	var sess *session
	getSession := func() (*session, error) {
		if sess != nil {
			return sess, nil
		}
		created, err := a.session(ctx)
		if err != nil {
			return nil, err
		}
		sess = created
		return sess, nil
	}

	// Client-side key resolution populates the target parameter before the
	// normal parameter loop, so the request itself stays a single plain GET.
	if spec.keyResolution != nil {
		if keyFlag := cmd.Flags().Lookup(spec.keyResolution.flag); keyFlag != nil && keyFlag.Changed {
			keySession, err := getSession()
			if err != nil {
				return err
			}
			resolved, err := a.resolveKey(ctx, keySession, cmd, spec)
			if err != nil {
				return err
			}
			if err := cmd.Flags().Set(spec.keyResolution.target, resolved); err != nil {
				return &processError{err: err}
			}
		}
	}

	pathValues := make(map[string]string, len(spec.params))
	var query url.Values
	for _, param := range spec.params {
		flag := cmd.Flags().Lookup(param.flag)
		if flag == nil {
			return &processError{err: fmt.Errorf("internal error: flag --%s is not registered for %s", param.flag, spec.operationID)}
		}
		// Mutual exclusion is enforced by cobra, so at most one of the
		// canonical flag and its hidden alias can have been set.
		if !flag.Changed && param.alias != "" {
			if aliasFlag := cmd.Flags().Lookup(param.alias); aliasFlag != nil && aliasFlag.Changed {
				flag = aliasFlag
			}
		}
		value := flag.Value.String()
		if !flag.Changed {
			if param.required {
				return &usageError{err: fmt.Errorf("--%s is required", param.flag)}
			}
			continue
		}
		if err := validateParam(param, value); err != nil {
			return &usageError{err: err}
		}
		if param.in == paramPath {
			pathValues[param.name] = value
		} else {
			if query == nil {
				query = url.Values{}
			}
			query.Set(param.name, value)
		}
	}

	apiPath, err := expandPath(spec.path, pathValues)
	if err != nil {
		return &processError{err: err}
	}

	requestSession, err := getSession()
	if err != nil {
		return err
	}
	apiClient, err := a.readClient(ctx, requestSession, spec.public, spec.optionalAuth)
	if err != nil {
		return err
	}

	resp, err := apiClient.Do(ctx, client.Request{
		Method:      "GET",
		Path:        apiPath,
		Query:       query,
		OperationID: spec.operationID,
	})
	if err != nil {
		return err
	}

	if spec.binary {
		return writeBinary(cmd, resp)
	}
	switch spec.operationID {
	case "getOAuthIdToken":
		return writeRedactedJSON(cmd.OutOrStdout(), resp, "idToken")
	case "getGiteaIntegration":
		return writeRedactedJSON(cmd.OutOrStdout(), resp, "webhookSecret")
	}
	return writeJSONStream(cmd.OutOrStdout(), resp)
}

// readClient selects the unauthenticated client for public operations so they
// never touch the credential backend, the optional-auth client for a public
// operation that benefits from a credential, and the authenticated client
// otherwise.
func (a *app) readClient(ctx context.Context, sess *session, public, optionalAuth bool) (*client.Client, error) {
	if public && !optionalAuth {
		return sess.newUnauthenticatedClient()
	}
	return sess.newClient(ctx)
}

func validateParam(param readParam, value string) error {
	if param.in == paramPath && (value == "" || value == "." || value == "..") {
		return fmt.Errorf("--%s must identify a nonempty path segment other than '.' or '..'", param.flag)
	}
	if param.minLen > 0 && len(value) < param.minLen {
		return fmt.Errorf("--%s must be at least %d characters", param.flag, param.minLen)
	}
	if param.maxLen > 0 && len(value) > param.maxLen {
		return fmt.Errorf("--%s must be at most %d characters", param.flag, param.maxLen)
	}
	if len(param.enum) > 0 {
		for _, allowed := range param.enum {
			if value == allowed {
				return nil
			}
		}
		return fmt.Errorf("--%s must be one of %s", param.flag, strings.Join(param.enum, ", "))
	}
	if param.numeric && !numericPattern.MatchString(value) {
		return fmt.Errorf("--%s must be a non-negative integer", param.flag)
	}
	if param.pattern != "" {
		matched, err := regexp.MatchString(param.pattern, value)
		if err != nil {
			return fmt.Errorf("--%s is invalid", param.flag)
		}
		if !matched {
			return fmt.Errorf("--%s does not match the required format", param.flag)
		}
	}
	return nil
}

// expandPath substitutes each documented path parameter, escaping the value so
// it cannot introduce a path separator or query delimiter.
func expandPath(template string, values map[string]string) (string, error) {
	path := template
	for name, value := range values {
		placeholder := "{" + name + "}"
		if !strings.Contains(path, placeholder) {
			continue
		}
		path = strings.ReplaceAll(path, placeholder, url.PathEscape(value))
	}
	if strings.Contains(path, "{") {
		return "", fmt.Errorf("unresolved path parameter in %q", template)
	}
	return path, nil
}

// validateBinaryOutput rejects missing destinations and known overwrite conflicts
// before accessing credentials or issuing a request. O_EXCL still protects the
// eventual open against a destination created after this check.
func validateBinaryOutput(cmd *cobra.Command) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "" {
		return &usageError{err: errors.New("--output is required")}
	}
	force, _ := cmd.Flags().GetBool("force")
	if output == "-" || force {
		return nil
	}
	if _, err := os.Lstat(output); err == nil {
		return &usageError{err: fmt.Errorf("destination %q already exists; pass --force to overwrite", output)}
	} else if !errors.Is(err, os.ErrNotExist) {
		return &processError{err: err}
	}
	return nil
}

// writeBinary streams a binary 200 response to an explicit destination. It never
// writes to stdout unless the caller asked for "-", and it refuses to overwrite
// an existing file without --force.
func writeBinary(cmd *cobra.Command, resp *client.Response) error {
	defer func() { _ = resp.Body.Close() }()

	output, _ := cmd.Flags().GetString("output")
	if resp.Empty() {
		return nil
	}
	if output == "-" {
		if _, err := copyBinary(cmd.OutOrStdout(), resp.Body); err != nil {
			return err
		}
		return nil
	}

	force, _ := cmd.Flags().GetBool("force")
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(output, flags, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return &usageError{err: fmt.Errorf("destination %q already exists; pass --force to overwrite", output)}
		}
		return &processError{err: err}
	}

	if _, copyErr := copyBinary(file, resp.Body); copyErr != nil {
		_ = file.Close()
		_ = os.Remove(output)
		return copyErr
	}
	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(output)
		return &processError{err: closeErr}
	}
	return nil
}

func copyBinary(dst io.Writer, src io.Reader) (int64, error) {
	written, err := io.Copy(dst, src)
	if err == nil {
		return written, nil
	}
	if errors.Is(err, context.Canceled) {
		return written, err
	}
	if client.IsTimeout(err) {
		return written, &client.TimeoutError{Err: err}
	}
	return written, &processError{err: err}
}
