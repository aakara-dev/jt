package ui

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/tui"
)

const help = `Open the interactive UI.

Keys: space opens the leader menu, : the command line, ctrl+k the floating
search, ? lists every key. Running jt with no arguments does the same.`

const examples = `$ jt ui
$ jt ui mine          # start on a list: issues, mine, sprint, backlog, recent, ...
$ jt ui DATA-12       # start on an issue
$ jt DATA-12          # shorthand`

// NewCmdUI is the ui command.
func NewCmdUI() *cobra.Command {
	return &cobra.Command{
		Use:     "ui [LIST | ISSUE-KEY]",
		Short:   "Open the interactive UI",
		Long:    help,
		Example: examples,
		Aliases: []string{"tui"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return Run(args)
		},
		ValidArgs: tui.BuiltinListNames,
	}
}

// Run launches the TUI, optionally starting on a list or issue.
func Run(args []string) error {
	var opts tui.Options
	if len(args) > 0 {
		opts.Start = args[0]
	}
	if opts.Start == "" {
		opts.Start = viper.GetString("tui.start")
	}
	client := api.DefaultClient(viper.GetBool("debug"))
	if err := tui.Run(tui.NewJiraBackend(client), opts); err != nil {
		return fmt.Errorf("ui: %w", err)
	}
	return nil
}

// RunSearch launches the TUI with the floating search open on input.
func RunSearch(input string) error {
	opts := tui.Options{Start: viper.GetString("tui.start")}.WithSearch(input)
	client := api.DefaultClient(viper.GetBool("debug"))
	if err := tui.Run(tui.NewJiraBackend(client), opts); err != nil {
		return fmt.Errorf("ui: %w", err)
	}
	return nil
}
