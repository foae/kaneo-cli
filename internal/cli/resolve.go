package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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

// boardPageSize is the largest page the board endpoint serves. Kaneo 2.26 and
// later always paginate the board (50 tasks by default, at most 100), so a
// resolution must walk pages or it silently misses everything past page one.
const boardPageSize = 100

// resolveTaskNumber walks a project's whole board, including the archived and
// planned buckets, for the single task carrying the requested number.
//
// Pages are requested sorted by number ascending. Numbers are unique per
// project, so the order is total and undisturbed by status or position
// changes, and it ends the walk early: once a page holds a number above the
// target, no later page can hold the target. Deleting a lower-numbered task
// during the walk shifts the offsets and can hide the target for that one
// run; a rerun resolves it. Servers that ignore paging return one page with
// no or a single-page pagination object, and the walk degrades to one request.
func resolveTaskNumber(ctx context.Context, apiClient *client.Client, projectID, key string, number int) (string, error) {
	apiPath, err := expandPath("/task/tasks/{projectId}", map[string]string{"projectId": projectID})
	if err != nil {
		return "", &processError{err: err}
	}

	type boardTask struct {
		ID string `json:"id"`
		// The board schema allows a null number, so this must tolerate null.
		Number *float64 `json:"number"`
	}
	type boardPage struct {
		Data struct {
			Columns []struct {
				Tasks []boardTask `json:"tasks"`
			} `json:"columns"`
			ArchivedTasks []boardTask `json:"archivedTasks"`
			PlannedTasks  []boardTask `json:"plannedTasks"`
		} `json:"data"`
		// Absent on servers that predate board pagination.
		Pagination *struct {
			TotalPages float64 `json:"totalPages"`
		} `json:"pagination"`
	}

	// A task can appear in more than one bucket, so count distinct IDs.
	seen := make(map[string]bool)
	var matches []string
	// The page count is fixed by the first response so a later page cannot
	// extend the walk; it is the only bound when the target is never passed.
	totalPages := 1
	for page := 1; ; page++ {
		payload, err := readBoundedJSON(ctx, apiClient, client.Request{
			Method: "GET",
			Path:   apiPath,
			Query: url.Values{
				"page":      []string{strconv.Itoa(page)},
				"limit":     []string{strconv.Itoa(boardPageSize)},
				"sortBy":    []string{"number"},
				"sortOrder": []string{"asc"},
			},
			OperationID: "listTasks",
		}, "task list")
		if err != nil {
			return "", err
		}
		var board boardPage
		if err := json.Unmarshal(payload, &board); err != nil {
			return "", &processError{err: client.ErrInvalidJSONResponse}
		}

		buckets := [][]boardTask{board.Data.ArchivedTasks, board.Data.PlannedTasks}
		for _, column := range board.Data.Columns {
			buckets = append(buckets, column.Tasks)
		}
		highest := 0.0
		for _, bucket := range buckets {
			for _, task := range bucket {
				if task.Number != nil && *task.Number > highest {
					highest = *task.Number
				}
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

		if page == 1 && board.Pagination != nil {
			totalPages = pageCount(board.Pagination.TotalPages)
		}
		// Stop at the server's last page or once the ascending order has
		// passed the target. A page with no bucketed task is not the end: a
		// task whose status matches no column occupies a slot without
		// appearing anywhere. A duplicate number is impossible upstream but is
		// still reported as ambiguous, so the walk never stops on a bare match.
		if page >= totalPages || highest > float64(number) {
			break
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

// pageCount converts a reported totalPages into a usable page count: anything
// that is not a finite number of at least one page is treated as one page.
func pageCount(reported float64) int {
	const maxPages = 1 << 30
	if math.IsNaN(reported) || reported < 1 {
		return 1
	}
	if reported > maxPages {
		return maxPages
	}
	return int(reported)
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
