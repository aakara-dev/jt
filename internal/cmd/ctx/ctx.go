package ctx

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/cmdutil"
	jtConfig "github.com/aakara-dev/jt/internal/config"
)

// SkipTokenAnnotation marks commands that work without a Jira API token.
const SkipTokenAnnotation = "jt/skip-token"

const help = `Contexts are named project/board pairs on your Jira server, like
kubectl/k9s contexts. Every command runs against the current context unless
you pass --context or set JT_CONTEXT.`

const examples = `# List contexts (current one is marked with *)
$ jt ctx

# Switch context, then switch back
$ jt ctx ops
$ jt ctx -

# Add a context; omit --project/--board to pick interactively
$ jt ctx add data --project DATA --board "Data Platform"

# Run a single command in another context
$ jt issue list --context ops`

// NewCmdCtx is the ctx command.
func NewCmdCtx() *cobra.Command {
	cmd := cobra.Command{
		Use:         "ctx [NAME | -]",
		Short:       "List and switch contexts (project/board pairs)",
		Long:        help,
		Example:     examples,
		Aliases:     []string{"context", "contexts"},
		Annotations: map[string]string{SkipTokenAnnotation: "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return list(cmd.OutOrStdout())
			}
			return use(cmd.OutOrStdout(), args[0])
		},
		ValidArgsFunction: CompleteContexts,
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
	return &cobra.Command{
		Use:         "list",
		Short:       "List contexts",
		Aliases:     []string{"ls"},
		Annotations: map[string]string{SkipTokenAnnotation: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return list(cmd.OutOrStdout())
		},
	}
}

func newCmdUse() *cobra.Command {
	return &cobra.Command{
		Use:               "use NAME | -",
		Short:             "Switch the current context ('-' switches back)",
		Aliases:           []string{"switch", "sw"},
		Annotations:       map[string]string{SkipTokenAnnotation: "true"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: CompleteContexts,
		RunE: func(cmd *cobra.Command, args []string) error {
			return use(cmd.OutOrStdout(), args[0])
		},
	}
}

func newCmdCurrent() *cobra.Command {
	return &cobra.Command{
		Use:         "current",
		Short:       "Print the current context name",
		Annotations: map[string]string{SkipTokenAnnotation: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, err := loadFile()
			if err != nil {
				return err
			}
			cur := f.GetString(jtConfig.KeyCurrentContext)
			if cur == "" {
				return fmt.Errorf("no current context set")
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), cur)
			return err
		},
	}
}

func newCmdAdd() *cobra.Command {
	cmd := cobra.Command{
		Use:   "add NAME",
		Short: "Add a context for a project and board",
		Long:  "Add a context. Missing --project/--board values are picked interactively.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := jtConfig.ValidateContextName(name); err != nil {
				return err
			}
			project, _ := cmd.Flags().GetString("project")
			board, _ := cmd.Flags().GetString("board")
			force, _ := cmd.Flags().GetBool("force")
			noSwitch, _ := cmd.Flags().GetBool("no-switch")

			f, err := loadFile()
			if err != nil {
				return err
			}
			if f.HasContext(name) && !force {
				return fmt.Errorf("context %q already exists, use --force to overwrite", name)
			}

			debug := viper.GetBool("debug")
			value, err := jtConfig.GenerateContext(api.DefaultClient(debug), project, board)
			if err != nil {
				return err
			}
			if err := f.Set(value, jtConfig.KeyContexts, name); err != nil {
				return err
			}
			if !noSwitch {
				if _, err := f.UseContext(name); err != nil {
					return err
				}
			}
			if err := f.Save(); err != nil {
				return err
			}
			cmdutil.Success("Added context %q", name)
			if !noSwitch {
				cmdutil.Success("Switched to context %q", name)
			}
			return nil
		},
	}
	// --project/-p is the root persistent flag and is read directly above.
	cmd.Flags().StringP("board", "b", "", "Board name for the context (\"None\" for no board)")
	cmd.Flags().BoolP("force", "f", false, "Overwrite an existing context")
	cmd.Flags().Bool("no-switch", false, "Do not switch to the new context")
	return &cmd
}

func newCmdRemove() *cobra.Command {
	return &cobra.Command{
		Use:               "remove NAME",
		Short:             "Remove a context",
		Aliases:           []string{"rm", "delete"},
		Annotations:       map[string]string{SkipTokenAnnotation: "true"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: CompleteContexts,
		RunE: func(_ *cobra.Command, args []string) error {
			f, err := loadFile()
			if err != nil {
				return err
			}
			if err := f.DeleteContext(args[0]); err != nil {
				return err
			}
			if err := f.Save(); err != nil {
				return err
			}
			cmdutil.Success("Removed context %q", args[0])
			return nil
		},
	}
}

func newCmdRename() *cobra.Command {
	return &cobra.Command{
		Use:               "rename OLD NEW",
		Short:             "Rename a context",
		Aliases:           []string{"mv"},
		Annotations:       map[string]string{SkipTokenAnnotation: "true"},
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: CompleteContexts,
		RunE: func(_ *cobra.Command, args []string) error {
			f, err := loadFile()
			if err != nil {
				return err
			}
			if err := f.RenameContext(args[0], args[1]); err != nil {
				return err
			}
			if err := f.Save(); err != nil {
				return err
			}
			cmdutil.Success("Renamed context %q to %q", args[0], args[1])
			return nil
		},
	}
}

func list(w io.Writer) error {
	f, err := loadFile()
	if err != nil {
		return err
	}
	ctxs := f.Contexts()
	if len(ctxs) == 0 {
		cmdutil.Warn("No contexts configured. Add one with: jt ctx add NAME")
		return nil
	}

	cur := color.New(color.FgGreen, color.Bold)
	dim := color.New(color.Faint)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, dim.Sprint("CURRENT\tNAME\tPROJECT\tBOARD"))
	for _, c := range ctxs {
		board := c.Board
		if board == "" {
			board = "-"
		} else if c.BoardID != "" {
			board = fmt.Sprintf("%s (#%s)", board, c.BoardID)
		}
		if c.Current {
			_, _ = fmt.Fprintln(tw, cur.Sprintf("*\t%s\t%s\t%s", c.Name, c.Project, board))
			continue
		}
		_, _ = fmt.Fprintf(tw, " \t%s\t%s\t%s\n", c.Name, c.Project, board)
	}
	return tw.Flush()
}

func use(w io.Writer, name string) error {
	f, err := loadFile()
	if err != nil {
		return err
	}
	name, err = f.UseContext(name)
	if err != nil {
		return err
	}
	if err := f.Save(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Switched to context %s\n", color.New(color.FgGreen, color.Bold).Sprint(name))
	return err
}

func loadFile() (*jtConfig.File, error) {
	path := viper.ConfigFileUsed()
	if path == "" {
		var err error
		if path, err = jtConfig.Path(""); err != nil {
			return nil, err
		}
	}
	return jtConfig.LoadFile(path)
}

// CompleteContexts completes context names for shell completion.
func CompleteContexts(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	f, err := loadFile()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	used := map[string]bool{}
	for _, a := range args {
		used[a] = true
	}
	var out []string
	for _, c := range f.Contexts() {
		if used[c.Name] {
			continue
		}
		out = append(out, fmt.Sprintf("%s\t%s", c.Name, c.Project))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
