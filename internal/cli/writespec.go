package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foae/kaneo-cli/internal/client"
)

// maxRequestBody bounds a body supplied on stdin or read from a file so a
// mistyped path cannot exhaust memory.
const maxRequestBody = 8 << 20

// writeSpec is one mutating operation mapped to a CLI command. The request body
// is supplied by the caller through --body-file and passed through unchanged
// after JSON validation, so unknown fields and numeric precision survive.
type writeSpec struct {
	group       string
	action      string
	short       string
	operationID string
	method      string
	path        string
	params      []readParam
	// body marks an operation with a documented JSON request body.
	body bool
	// destructive marks a delete/remove/revoke/reset-equivalent operation that
	// requires --yes before any request.
	destructive bool
	// public marks an operation whose documented security is empty.
	public bool
}

func (a *app) newWriteCommand(spec writeSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   spec.action,
		Short: spec.short,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runWrite(cmd, spec)
		},
	}
	if bodyHelp := generatedBodyHelp[spec.operationID]; bodyHelp != "" {
		cmd.Long = spec.short + "\n\n" + bodyHelp
	}
	for _, param := range spec.params {
		help := param.help
		if len(param.enum) > 0 {
			help = strings.TrimSpace(help + " (one of: " + strings.Join(param.enum, ", ") + ")")
		}
		if param.required {
			help = strings.TrimSpace(help + " (required)")
		}
		cmd.Flags().String(param.flag, "", help)
	}
	if spec.body {
		cmd.Flags().String("body-file", "", "Path to a JSON request body, or - for stdin (required)")
	}
	return cmd
}

func (a *app) runWrite(cmd *cobra.Command, spec writeSpec) error {
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

	// The confirmation gate runs before any credential or network work.
	if spec.destructive && !a.flags.yes {
		return &usageError{err: fmt.Errorf("refusing to run %s without --yes", spec.operationID)}
	}

	var body []byte
	if spec.body {
		body, err = a.readBody(cmd)
		if err != nil {
			return err
		}
	}

	sess, err := a.session(ctx)
	if err != nil {
		return err
	}
	apiClient, err := a.readClient(ctx, sess, spec.public, false)
	if err != nil {
		return err
	}

	resp, err := apiClient.Do(ctx, client.Request{
		Method:      spec.method,
		Path:        apiPath,
		Query:       query,
		Body:        body,
		ContentType: contentTypeFor(body),
		OperationID: spec.operationID,
		Sensitive:   !spec.public && body != nil,
	})
	if err != nil {
		return err
	}
	return writeMutationResult(cmd.OutOrStdout(), resp, spec.operationID)
}

// Preserve the complete server result on stdout even when a batch only partly
// succeeds. Scripts can inspect individual outcomes while relying on exit 5.
func writeMutationResult(out io.Writer, resp *client.Response, operation string) error {
	switch operation {
	case "bulkUpdateTasks", "importTasks", "importGitHubIssues", "importGiteaIssues":
	default:
		return writeJSONStream(out, resp)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRequestBody+1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if client.IsTimeout(err) {
			return &client.TimeoutError{Err: err}
		}
		return &processError{err: err}
	}
	if len(payload) > maxRequestBody {
		return &processError{err: errors.New("response exceeds size limit")}
	}
	var result struct {
		Success *bool `json:"success"`
		Results struct {
			Failed int `json:"failed"`
		} `json:"results"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return client.ErrInvalidJSONResponse
	}
	if err := writeJSONBytes(out, payload); err != nil {
		return err
	}
	if (result.Success != nil && !*result.Success) || result.Results.Failed > 0 || len(result.Errors) > 0 {
		return &client.Error{
			StatusCode: resp.StatusCode, Code: "partial_failure",
			Message:     "operation reported failed items; inspect the result on stdout",
			OperationID: operation,
		}
	}
	return nil
}

// readBody reads and validates the JSON request body before any network access.
func (a *app) readBody(cmd *cobra.Command) ([]byte, error) {
	path, _ := cmd.Flags().GetString("body-file")
	if path == "" {
		return nil, &usageError{err: errors.New("--body-file is required")}
	}
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxRequestBody+1))
		if err != nil {
			return nil, &processError{err: err}
		}
		if len(data) > maxRequestBody {
			return nil, &usageError{err: fmt.Errorf("request body exceeds %d bytes", maxRequestBody)}
		}
	} else {
		data, err = readBodyFile(path)
		if err != nil {
			return nil, err
		}
	}
	if err := validateJSONBody(data); err != nil {
		return nil, err
	}
	return data, nil
}

// validateJSONBody requires a JSON object, rejects an empty body and leaves the
// bytes untouched so unknown fields and numeric precision are preserved.
func validateJSONBody(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return &usageError{err: errors.New("request body is empty; supply a JSON object")}
	}
	if !json.Valid(data) {
		return &usageError{err: errors.New("request body is not valid JSON")}
	}
	if !strings.HasPrefix(trimmed, "{") {
		return &usageError{err: errors.New("request body must be a JSON object")}
	}
	return nil
}

func contentTypeFor(body []byte) string {
	if body == nil {
		return ""
	}
	return "application/json"
}
