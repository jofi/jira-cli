package worklog

import (
	"github.com/spf13/cobra"

	"github.com/ankitpokhrel/jira-cli/internal/cmd/issue/worklog/add"
)

const helpText = `Worklog command helps you manage issue comments. See available commands below.`

// NewCmdWorklog is a worklog command.
func NewCmdWorklog() *cobra.Command {
	cmd := cobra.Command{
		Use:     "worklog",
		Short:   "Manage issue worklogs",
		Long:    helpText,
		Aliases: []string{"worklogs"},
		RunE:    worklog,
	}

	cmd.AddCommand(add.NewCmdCWorklogAdd())

	return &cmd
}

func worklog(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
