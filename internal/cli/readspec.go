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
	help     string
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
	// binary marks an operation whose 200 response is a non-JSON stream that
	// must be written to an explicit destination.
	binary bool
}

func (a *app) newReadCommandGroups() []*cobra.Command {
	order := make([]string, 0)
	byGroup := make(map[string][]readSpec)
	for _, spec := range readSpecs {
		if _, seen := byGroup[spec.group]; !seen {
			order = append(order, spec.group)
		}
		byGroup[spec.group] = append(byGroup[spec.group], spec)
	}

	groups := make([]*cobra.Command, 0, len(order))
	for _, name := range order {
		group := &cobra.Command{
			Use:   name,
			Short: groupShorts[name],
			Args:  noArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return help(cmd)
			},
		}
		for _, spec := range byGroup[name] {
			group.AddCommand(a.newReadCommand(spec))
		}
		groups = append(groups, group)
	}
	return groups
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
		cmd.Flags().String(param.flag, "", help)
	}
	if spec.binary {
		cmd.Flags().String("output", "", "Destination file path, or - for stdout (required)")
		cmd.Flags().Bool("force", false, "Overwrite an existing destination file")
	}
	return cmd
}

func (a *app) runRead(cmd *cobra.Command, spec readSpec) error {
	ctx := cmd.Context()

	pathValues := make(map[string]string, len(spec.params))
	var query url.Values
	for _, param := range spec.params {
		flag := cmd.Flags().Lookup(param.flag)
		if flag == nil {
			return &processError{err: fmt.Errorf("internal error: flag --%s is not registered for %s", param.flag, spec.operationID)}
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

	sess, err := a.session(ctx)
	if err != nil {
		return err
	}
	apiClient, err := a.readClient(ctx, sess, spec.public)
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
	return writeJSONStream(cmd.OutOrStdout(), resp)
}

// readClient selects the unauthenticated client for public operations so they
// never touch the credential backend, and the authenticated client otherwise.
func (a *app) readClient(ctx context.Context, sess *session, public bool) (*client.Client, error) {
	if public {
		return sess.newUnauthenticatedClient()
	}
	return sess.newClient(ctx)
}

func validateParam(param readParam, value string) error {
	if param.minLen > 0 && len(value) < param.minLen {
		return fmt.Errorf("--%s must be at least %d characters", param.flag, param.minLen)
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

// writeBinary streams a binary 200 response to an explicit destination. It never
// writes to stdout unless the caller asked for "-", and it refuses to overwrite
// an existing file without --force.
func writeBinary(cmd *cobra.Command, resp *client.Response) error {
	defer func() { _ = resp.Body.Close() }()

	output, _ := cmd.Flags().GetString("output")
	if output == "" {
		return &usageError{err: errors.New("--output is required")}
	}
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
