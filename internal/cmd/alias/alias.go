package alias

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aakara-dev/jt/internal/cmdutil"
	jtConfig "github.com/aakara-dev/jt/internal/config"
	"github.com/aakara-dev/jt/internal/theme"
)

const help = `Aliases are shortcuts for jt commands, stored under aliases: in the config.

Expansions may use:
  {me} {project} {board}  your login and the active project's key/board
  $1 $2 ...               positional arguments; unused arguments are appended
  !<shell command>        run through sh, arguments are available as "$@"`

const examples = `$ jt alias set blocked 'issue list -s Blocked --order-by updated'
$ jt alias set start 'issue move $1 "In Progress"'      # jt start DATA-12
$ jt alias set pr '!jt issue view "$1" --plain | head -20'
$ jt alias set mine ''                                   # disable a default
$ jt alias ls`

// NewCmdAlias is the alias command.
func NewCmdAlias() *cobra.Command {
	cmd := cobra.Command{
		Use:         "alias",
		Short:       "Manage command aliases",
		Long:        help,
		Example:     examples,
		Aliases:     []string{"aliases"},
		Annotations: map[string]string{cmdutil.SkipTokenAnnotation: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return list(cmd.OutOrStdout())
		},
	}
	cmd.AddCommand(newCmdList(), newCmdSet(), newCmdDelete())
	return &cmd
}

func newCmdList() *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List aliases",
		Aliases:     []string{"ls"},
		Annotations: map[string]string{cmdutil.SkipTokenAnnotation: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return list(cmd.OutOrStdout())
		},
	}
}

func newCmdSet() *cobra.Command {
	return &cobra.Command{
		Use:         "set NAME EXPANSION",
		Short:       "Create or update an alias",
		Annotations: map[string]string{cmdutil.SkipTokenAnnotation: "true"},
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, expansion := args[0], args[1]
			if strings.ContainsAny(name, " \t'\"") || strings.HasPrefix(name, "-") {
				return fmt.Errorf("invalid alias name %q", name)
			}
			if IsBuiltin(cmd.Root(), name) {
				return fmt.Errorf("%q is already a jt command", name)
			}
			f, err := jtConfig.LoadCurrent()
			if err != nil {
				return err
			}
			if err := f.Set(expansion, jtConfig.KeyAliases, name); err != nil {
				return err
			}
			if err := f.Save(); err != nil {
				return err
			}
			if strings.TrimSpace(expansion) == "" {
				cmdutil.Success("Disabled alias %q", name)
			} else {
				cmdutil.Success("Alias %q now expands to: %s", name, expansion)
			}
			return nil
		},
	}
}

func newCmdDelete() *cobra.Command {
	return &cobra.Command{
		Use:         "delete NAME",
		Short:       "Delete a user alias (defaults come back)",
		Aliases:     []string{"rm", "remove"},
		Annotations: map[string]string{cmdutil.SkipTokenAnnotation: "true"},
		Args:        cobra.ExactArgs(1),
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			f, err := jtConfig.LoadCurrent()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return f.Keys(jtConfig.KeyAliases), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(_ *cobra.Command, args []string) error {
			f, err := jtConfig.LoadCurrent()
			if err != nil {
				return err
			}
			if !f.Delete(jtConfig.KeyAliases, args[0]) {
				return fmt.Errorf("no user alias %q", args[0])
			}
			if err := f.Save(); err != nil {
				return err
			}
			cmdutil.Success("Deleted alias %q", args[0])
			return nil
		},
	}
}

func list(w io.Writer) error {
	f, err := jtConfig.LoadCurrent()
	if err != nil {
		return err
	}
	user := f.StringMap(jtConfig.KeyAliases)
	merged := Merge(user)

	st := theme.Active()
	rows := make([][]string, 0, len(merged))
	for _, n := range Names(merged) {
		src := "user"
		if _, ok := user[n]; !ok {
			src = "default"
		}
		rows = append(rows, []string{st.Key.Render(n), merged[n], st.Muted.Render(src)})
	}
	return theme.Table(w, st, []string{"ALIAS", "EXPANSION", "SOURCE"}, rows, -1)
}
