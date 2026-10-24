package find

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/cmd/ui"
	"github.com/aakara-dev/jt/internal/cmdutil"
	"github.com/aakara-dev/jt/internal/theme"
	"github.com/aakara-dev/jt/internal/tui"
)

const help = `Search issues with the same syntax as the floating search in the UI.

  text        summary, description and comments in the current project
  >JQL        raw JQL
  #KEY  #12   an issue key (#12 uses the current project's key)
  @user @me   assigned to or reported by a user
  *name       a saved query from saved_queries
  !text       text search across all saved projects`

const examples = `$ jt find retry loader
$ jt find '#123'
$ jt find '@me' --limit 10
$ jt find '!orders' -i      # open the results in the floating search
$ jt find '>status = Blocked' --jql`

const defaultLimit = 25

// NewCmdFind is the find command.
func NewCmdFind() *cobra.Command {
	cmd := cobra.Command{
		Use:     "find QUERY...",
		Short:   "Search issues (text, >jql, #key, @user, *saved, !everywhere)",
		Long:    help,
		Example: examples,
		Aliases: []string{"s", "search"},
		Args:    cobra.MinimumNArgs(1),
		RunE:    run,
	}
	cmd.Flags().BoolP("interactive", "i", false, "Open the floating search in the UI")
	cmd.Flags().UintP("limit", "n", defaultLimit, "Maximum number of results")
	cmd.Flags().Bool("jql", false, "Print the generated JQL instead of searching")
	return &cmd
}

func run(cmd *cobra.Command, args []string) error {
	input := strings.Join(args, " ")
	if interactive, _ := cmd.Flags().GetBool("interactive"); interactive {
		return ui.RunSearch(input)
	}

	sess := tui.LoadSession()
	jql := tui.SearchJQL(sess, input)
	if jql == "" {
		return fmt.Errorf("nothing to search for %q", input)
	}
	if printJQL, _ := cmd.Flags().GetBool("jql"); printJQL {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), jql)
		return err
	}

	limit, _ := cmd.Flags().GetUint("limit")
	s := cmdutil.Info("Searching...")
	res, err := api.ProxySearch(api.DefaultClient(viper.GetBool("debug")), jql, 0, limit)
	s.Stop()
	if err != nil {
		return err
	}
	if len(res.Issues) == 0 {
		cmdutil.Warn("No issues found for: %s", jql)
		return nil
	}

	st := theme.Active()
	p := st.Palette
	rows := make([][]string, 0, len(res.Issues))
	for _, iss := range res.Issues {
		f := iss.Fields
		status := f.Status.Category.Key
		if status == "" {
			status = f.Status.Name
		}
		rows = append(rows, []string{
			st.Accent.Render(iss.Key),
			f.IssueType.Name,
			st.Value.Foreground(p.Status(status)).Render(f.Status.Name),
			f.Assignee.Name,
			f.Summary,
		})
	}
	if err := theme.Table(cmd.OutOrStdout(), st, []string{"KEY", "TYPE", "STATUS", "ASSIGNEE", "SUMMARY"}, rows, -1); err != nil {
		return err
	}
	if res.Total > len(res.Issues) {
		theme.Fprintln(cmd.ErrOrStderr(), st.Muted.Render(fmt.Sprintf("%d of %d · use --limit or -i for more", len(res.Issues), res.Total)))
	}
	return nil
}
