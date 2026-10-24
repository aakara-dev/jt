package project

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/cmd/project/list"
	"github.com/aakara-dev/jt/internal/cmdutil"
	jtConfig "github.com/aakara-dev/jt/internal/config"
	"github.com/aakara-dev/jt/internal/theme"
)

const helpText = `Saved projects are named project/board pairs you switch between, like
k9s contexts. Every command runs against the current project unless you pass
-p/--project (a saved name or any Jira key) or set JT_PROJECT.`

const examples = `# List saved projects (current one is marked with *)
$ jt project

# Switch, then switch back
$ jt project ops
$ jt project -

# Save a project; omit KEY or --board to pick interactively
$ jt project add data DATA --board "Data Platform"

# Run one command in another project
$ jt issue list -p ops

# Every project on the server
$ jt project ls --server`

var skipToken = map[string]string{cmdutil.SkipTokenAnnotation: "true"}

// NewCmdProject is the project command.
func NewCmdProject() *cobra.Command {
	cmd := cobra.Command{
		Use:               "project [NAME | -]",
		Short:             "List and switch saved projects",
		Long:              helpText,
		Example:           examples,
		Aliases:           []string{"projects", "proj", "p", "ctx"},
		Annotations:       map[string]string{"cmd:main": "true", cmdutil.SkipTokenAnnotation: "true"},
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: CompleteProjects,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return listSaved(cmd.OutOrStdout())
			}
			return use(cmd.OutOrStdout(), args[0])
		},
	}

	cmd.AddCommand(
		newCmdList(),
		newCmdUse(),
		newCmdCurrent(),
		newCmdAdd(),
		newCmdRemove(),
		newCmdRename(),
	)
	return &cmd
}

func newCmdList() *cobra.Command {
	cmd := cobra.Command{
		Use:         "list",
		Short:       "List saved projects, or all server projects with --server",
		Aliases:     []string{"lists", "ls"},
		Annotations: skipToken, // listing saved projects works offline
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if server, _ := cmd.Flags().GetBool("server"); server {
				list.List(cmd, args)
				return nil
			}
			return listSaved(cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolP("server", "s", false, "List every project on the Jira server you can access")
	return &cmd
}

func newCmdUse() *cobra.Command {
	return &cobra.Command{
		Use:               "use NAME | -",
		Short:             "Switch the current project ('-' switches back)",
		Aliases:           []string{"switch", "sw"},
		Annotations:       skipToken,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: CompleteProjects,
		RunE: func(cmd *cobra.Command, args []string) error {
			return use(cmd.OutOrStdout(), args[0])
		},
	}
}

func newCmdCurrent() *cobra.Command {
	return &cobra.Command{
		Use:         "current",
		Short:       "Print the current saved project name",
		Annotations: skipToken,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, err := jtConfig.LoadCurrent()
			if err != nil {
				return err
			}
			cur := f.GetString(jtConfig.KeyCurrentProject)
			if cur == "" {
				return fmt.Errorf("no current project set")
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), cur)
			return err
		},
	}
}

func newCmdAdd() *cobra.Command {
	cmd := cobra.Command{
		Use:   "add NAME [KEY]",
		Short: "Save a project (and board) under a short name",
		Long:  "Save a project. A missing KEY or --board is picked interactively.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := jtConfig.ValidateProjectName(name); err != nil {
				return err
			}
			var key string
			if len(args) == 2 {
				key = args[1]
			}
			board, _ := cmd.Flags().GetString("board")
			force, _ := cmd.Flags().GetBool("force")
			noSwitch, _ := cmd.Flags().GetBool("no-switch")

			f, err := jtConfig.LoadCurrent()
			if err != nil {
				return err
			}
			if f.HasProject(name) && !force {
				return fmt.Errorf("project %q already exists, use --force to overwrite", name)
			}

			value, err := jtConfig.GenerateProject(api.DefaultClient(viper.GetBool("debug")), key, board)
			if err != nil {
				return err
			}
			if err := f.Set(value, jtConfig.KeyProjects, name); err != nil {
				return err
			}
			if !noSwitch {
				if _, err := f.UseProject(name); err != nil {
					return err
				}
			}
			if err := f.Save(); err != nil {
				return err
			}
			cmdutil.Success("Saved project %q", name)
			if !noSwitch {
				cmdutil.Success("Switched to project %q", name)
			}
			return nil
		},
	}
	cmd.Flags().StringP("board", "b", "", "Board name (\"None\" for no board)")
	cmd.Flags().BoolP("force", "f", false, "Overwrite an existing saved project")
	cmd.Flags().Bool("no-switch", false, "Do not switch to the new project")
	return &cmd
}

func newCmdRemove() *cobra.Command {
	return &cobra.Command{
		Use:               "remove NAME",
		Short:             "Remove a saved project",
		Aliases:           []string{"rm", "delete"},
		Annotations:       skipToken,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: CompleteProjects,
		RunE: func(_ *cobra.Command, args []string) error {
			f, err := jtConfig.LoadCurrent()
			if err != nil {
				return err
			}
			if err := f.DeleteProject(args[0]); err != nil {
				return err
			}
			if err := f.Save(); err != nil {
				return err
			}
			cmdutil.Success("Removed project %q", args[0])
			return nil
		},
	}
}

func newCmdRename() *cobra.Command {
	return &cobra.Command{
		Use:               "rename OLD NEW",
		Short:             "Rename a saved project",
		Aliases:           []string{"mv"},
		Annotations:       skipToken,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: CompleteProjects,
		RunE: func(_ *cobra.Command, args []string) error {
			f, err := jtConfig.LoadCurrent()
			if err != nil {
				return err
			}
			if err := f.RenameProject(args[0], args[1]); err != nil {
				return err
			}
			if err := f.Save(); err != nil {
				return err
			}
			cmdutil.Success("Renamed project %q to %q", args[0], args[1])
			return nil
		},
	}
}

func listSaved(w io.Writer) error {
	f, err := jtConfig.LoadCurrent()
	if err != nil {
		return err
	}
	projects := f.Projects()
	if len(projects) == 0 {
		cmdutil.Warn("No saved projects. Add one with: jt project add NAME [KEY]")
		return nil
	}

	rows := make([][]string, 0, len(projects))
	current := -1
	for i, p := range projects {
		board := p.Board
		if board == "" {
			board = "-"
		} else if p.BoardID != "" {
			board = fmt.Sprintf("%s (#%s)", board, p.BoardID)
		}
		marker := " "
		if p.Current {
			marker, current = "*", i
		}
		rows = append(rows, []string{marker, p.Name, p.Key, board})
	}
	return theme.Table(w, theme.Active(), []string{"", "NAME", "KEY", "BOARD"}, rows, current)
}

func use(w io.Writer, selector string) error {
	f, err := jtConfig.LoadCurrent()
	if err != nil {
		return err
	}
	name := selector
	if selector != "-" {
		if name = f.Resolve(selector); name == "" {
			return fmt.Errorf("project %q not found (see 'jt project ls', or save it with 'jt project add')", selector)
		}
	}
	name, err = f.UseProject(name)
	if err != nil {
		return err
	}
	if err := f.Save(); err != nil {
		return err
	}
	st := theme.Active()
	theme.Fprintln(w, st.Muted.Render("Switched to project"), st.Selected.Render(name))
	return nil
}

// CompleteProjects completes saved project names for shell completion.
func CompleteProjects(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	f, err := jtConfig.LoadCurrent()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	used := map[string]bool{}
	for _, a := range args {
		used[a] = true
	}
	var out []string
	for _, p := range f.Projects() {
		if !used[p.Name] {
			out = append(out, fmt.Sprintf("%s\t%s", p.Name, p.Key))
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
