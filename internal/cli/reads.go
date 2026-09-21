package cli

import (
	"github.com/spf13/cobra"

	"github.com/foae/kaneo-cli/internal/client"
)

func (a *app) newInstanceCommand() *cobra.Command {
	group := &cobra.Command{
		Use:   "instance",
		Short: "Public instance information",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return help(cmd)
		},
	}
	group.AddCommand(&cobra.Command{
		Use:   "get-status",
		Short: "Get public instance setup status",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.getJSON(cmd, "/instance/status", "getInstanceStatus")
		},
	})
	return group
}

func (a *app) newConfigCommand() *cobra.Command {
	group := &cobra.Command{
		Use:   "config",
		Short: "Public instance configuration",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return help(cmd)
		},
	}
	group.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Get public instance settings",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.getJSON(cmd, "/config", "getConfig")
		},
	})
	return group
}

// getJSON performs a GET whose response is passed through to stdout.
func (a *app) getJSON(cmd *cobra.Command, path, operationID string) error {
	ctx := cmd.Context()
	sess, err := a.session(ctx)
	if err != nil {
		return err
	}
	// Public operations carry no credential by contract; loading one could block
	// on an unavailable backend for no benefit.
	apiClient, err := sess.newUnauthenticatedClient()
	if err != nil {
		return err
	}
	resp, err := apiClient.Do(ctx, client.Request{
		Method:      "GET",
		Path:        path,
		OperationID: operationID,
	})
	if err != nil {
		return err
	}
	return writeJSONStream(cmd.OutOrStdout(), resp)
}
