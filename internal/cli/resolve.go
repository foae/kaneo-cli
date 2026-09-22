package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foae/kaneo-cli/internal/client"
)

// resolveKey reads the client-side key flags and resolves them into the opaque
// identifier the spec parameter expects. Only task keys are supported today.
func (a *app) resolveKey(ctx context.Context, sess *session, cmd *cobra.Command, spec readSpec) (string, error) {
	key, _ := cmd.Flags().GetString(spec.keyResolution.flag)
	workspaceID, _ := cmd.Flags().GetString("workspace-id")

	// Cobra only enforces that the flags appear together, so an empty value
	// must be rejected here rather than sent to the API as a blank filter.
	if key == "" {
		return "", &usageError{err: fmt.Errorf("--%s is required", spec.keyResolution.flag)}
	}
	if workspaceID == "" {
		return "", &usageError{err: fmt.Errorf("--workspace-id is required with --%s", spec.keyResolution.flag)}
	}
	return a.resolveTaskKey(ctx, sess, workspaceID, key)
}

// resolveTaskKey turns a display key such as "KAN-12" into a task ID by looking
// up the project slug in the workspace and then the task number on its board.
func (a *app) resolveTaskKey(ctx context.Context, sess *session, workspaceID, key string) (string, error) {
	slug, number, err := parseTaskKey(key)
	if err != nil {
		return "", err
	}
	apiClient, err := sess.newClient(ctx)
	if err != nil {
		return "", err
	}

	projectID, err := resolveProjectSlug(ctx, apiClient, workspaceID, slug)
	if err != nil {
		return "", err
	}
	return resolveTaskNumber(ctx, apiClient, projectID, key, number)
}

// parseTaskKey splits a display key such as "KAN-12" into its project slug and
// task number. The split is on the last separator so a slug may contain one.
func parseTaskKey(key string) (string, int, error) {
	index := strings.LastIndex(key, "-")
	if index <= 0 || index == len(key)-1 {
		return "", 0, &usageError{err: fmt.Errorf("invalid task key %q; expected a project slug and number such as KAN-12", key)}
	}
	slug := key[:index]
	digits := key[index+1:]
	if !numericPattern.MatchString(digits) {
		return "", 0, &usageError{err: fmt.Errorf("invalid task key %q; expected a project slug and number such as KAN-12", key)}
	}
	number, err := strconv.Atoi(digits)
	if err != nil || number <= 0 {
		return "", 0, &usageError{err: fmt.Errorf("invalid task key %q; expected a project slug and number such as KAN-12", key)}
	}
	return slug, number, nil
}

// resolveProjectSlug finds the single project in a workspace whose slug matches,
// comparing case-insensitively because keys are commonly typed in either case.
func resolveProjectSlug(ctx context.Context, apiClient *client.Client, workspaceID, slug string) (string, error) {
	payload, err := readBoundedJSON(ctx, apiClient, client.Request{
		Method: "GET",
		Path:   "/project",
		// Archived projects are excluded by default, but the board scan below
		// includes archived tasks, so the project lookup must match.
		Query:       url.Values{"workspaceId": []string{workspaceID}, "includeArchived": []string{"true"}},
		OperationID: "listProjects",
	}, "project list")
	if err != nil {
		return "", err
	}
	var projects []struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(payload, &projects); err != nil {
		return "", &processError{err: client.ErrInvalidJSONResponse}
	}

	var matches []string
	for _, project := range projects {
		if strings.EqualFold(project.Slug, slug) && project.ID != "" {
			matches = append(matches, project.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", &usageError{err: fmt.Errorf("no project with slug %q in workspace %q", slug, workspaceID)}
	default:
		return "", &usageError{err: fmt.Errorf("project slug %q matches %d projects in workspace %q; use --id with the task ID instead", slug, len(matches), workspaceID)}
	}
}

// resolveTaskNumber scans a project's whole board, including the archived and
// planned buckets, for the single task carrying the requested number.
func resolveTaskNumber(ctx context.Context, apiClient *client.Client, projectID, key string, number int) (string, error) {
	apiPath, err := expandPath("/task/tasks/{projectId}", map[string]string{"projectId": projectID})
	if err != nil {
		return "", &processError{err: err}
	}
	payload, err := readBoundedJSON(ctx, apiClient, client.Request{
		Method:      "GET",
		Path:        apiPath,
		OperationID: "listTasks",
	}, "task list")
	if err != nil {
		return "", err
	}

	type boardTask struct {
		ID string `json:"id"`
		// The board schema allows a null number, so this must tolerate null.
		Number *float64 `json:"number"`
	}
	var board struct {
		Data struct {
			Columns []struct {
				Tasks []boardTask `json:"tasks"`
			} `json:"columns"`
			ArchivedTasks []boardTask `json:"archivedTasks"`
			PlannedTasks  []boardTask `json:"plannedTasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &board); err != nil {
		return "", &processError{err: client.ErrInvalidJSONResponse}
	}

	buckets := [][]boardTask{board.Data.ArchivedTasks, board.Data.PlannedTasks}
	for _, column := range board.Data.Columns {
		buckets = append(buckets, column.Tasks)
	}

	// A task can appear in more than one bucket, so count distinct IDs.
	seen := make(map[string]bool)
	var matches []string
	for _, bucket := range buckets {
		for _, task := range bucket {
			if task.ID == "" || task.Number == nil || *task.Number != float64(number) {
				continue
			}
			if seen[task.ID] {
				continue
			}
			seen[task.ID] = true
			matches = append(matches, task.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", &usageError{err: fmt.Errorf("no task %q in that project", key)}
	default:
		return "", &usageError{err: fmt.Errorf("task key %q matches %d tasks; use --id with the task ID instead", key, len(matches))}
	}
}

// readBoundedJSON issues a request and reads its body under the shared response
// limit, so a hostile or oversized board fails loudly instead of silently.
func readBoundedJSON(ctx context.Context, apiClient *client.Client, request client.Request, what string) ([]byte, error) {
	resp, err := apiClient.Do(ctx, request)
	if err != nil {
		return nil, err
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRequestBody+1))
	_ = resp.Body.Close()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		if client.IsTimeout(err) {
			return nil, &client.TimeoutError{Err: err}
		}
		return nil, &processError{err: err}
	}
	if len(payload) > maxRequestBody {
		return nil, &processError{err: fmt.Errorf("%s response exceeds %d bytes", what, maxRequestBody)}
	}
	return payload, nil
}
