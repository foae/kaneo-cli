package cli

import "github.com/spf13/cobra"

// groupedCommand attaches one command to a named group.
type groupedCommand struct {
	group string
	cmd   *cobra.Command
}

// newAPIGroups builds one command group per documented group, merging read,
// write and dedicated upload commands so a group is never registered twice.
func (a *app) newAPIGroups() []*cobra.Command {
	var order []string
	groups := make(map[string]*cobra.Command)
	add := func(group string, child *cobra.Command) {
		parent, ok := groups[group]
		if !ok {
			parent = &cobra.Command{
				Use:   group,
				Short: groupShorts[group],
				Args:  noArgs,
				RunE: func(cmd *cobra.Command, _ []string) error {
					return help(cmd)
				},
			}
			groups[group] = parent
			order = append(order, group)
		}
		parent.AddCommand(child)
	}

	for _, spec := range readSpecs {
		add(spec.group, a.newReadCommand(spec))
	}
	for _, spec := range writeSpecs {
		add(spec.group, a.newWriteCommand(spec))
	}
	for _, special := range a.newUploadCommands() {
		add(special.group, special.cmd)
	}
	for _, navigation := range a.newNavigationCommands() {
		add(navigation.group, navigation.cmd)
	}

	built := make([]*cobra.Command, 0, len(order))
	for _, group := range order {
		built = append(built, groups[group])
	}
	return built
}
