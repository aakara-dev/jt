package issue

import (
	"github.com/spf13/cobra"

	"github.com/aakara-dev/jt/internal/cmd/issue/assign"
	"github.com/aakara-dev/jt/internal/cmd/issue/clone"
	"github.com/aakara-dev/jt/internal/cmd/issue/comment"
	"github.com/aakara-dev/jt/internal/cmd/issue/create"
	"github.com/aakara-dev/jt/internal/cmd/issue/delete"
	"github.com/aakara-dev/jt/internal/cmd/issue/edit"
	"github.com/aakara-dev/jt/internal/cmd/issue/link"
	"github.com/aakara-dev/jt/internal/cmd/issue/list"
	"github.com/aakara-dev/jt/internal/cmd/issue/move"
	"github.com/aakara-dev/jt/internal/cmd/issue/unlink"
	"github.com/aakara-dev/jt/internal/cmd/issue/view"
	"github.com/aakara-dev/jt/internal/cmd/issue/watch"
	"github.com/aakara-dev/jt/internal/cmd/issue/worklog"
)

const helpText = `Issue manage issues in a given project. See available commands below.`

// NewCmdIssue is an issue command.
func NewCmdIssue() *cobra.Command {
	cmd := cobra.Command{
		Use:         "issue",
		Short:       "Issue manage issues in a project",
		Long:        helpText,
		Aliases:     []string{"issues"},
		Annotations: map[string]string{"cmd:main": "true"},
		RunE:        issue,
	}

	lc := list.NewCmdList()
	cc := create.NewCmdCreate()

	cmd.AddCommand(
		lc, cc, edit.NewCmdEdit(), move.NewCmdMove(), view.NewCmdView(), assign.NewCmdAssign(),
		link.NewCmdLink(), unlink.NewCmdUnlink(), comment.NewCmdComment(), clone.NewCmdClone(),
		delete.NewCmdDelete(), watch.NewCmdWatch(), worklog.NewCmdWorklog(),
	)

	list.SetFlags(lc)
	create.SetFlags(cc)

	return &cmd
}

func issue(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
