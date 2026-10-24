package standup

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/cmdutil"
	"github.com/aakara-dev/jt/pkg/jira"
)

const help = `Print a standup summary in markdown, ready to paste:

  Yesterday  issues you moved or logged work on since the last working day
  Today      your issues in progress
  Blockers   your open issues that are Blocked or flagged`

const examples = `$ jt standup
$ jt standup --copy          # also copy to the clipboard
$ jt standup --since 3d      # cover a longer gap
$ jt standup --links         # link keys to Jira`

// NewCmdStandup is the standup command.
func NewCmdStandup() *cobra.Command {
	cmd := cobra.Command{
		Use:     "standup",
		Short:   "Summarise yesterday, today and blockers",
		Long:    help,
		Example: examples,
		Aliases: []string{"st", "daily"},
		Args:    cobra.NoArgs,
		RunE:    run,
	}
	cmd.Flags().String("since", "", "How far back \"yesterday\" goes, e.g. 1d, 3d (default: start of the last working day)")
	cmd.Flags().BoolP("copy", "y", false, "Copy the summary to the clipboard")
	cmd.Flags().Bool("links", false, "Render issue keys as markdown links")
	return &cmd
}

// sinceClause returns a JQL date expression for the start of "yesterday".
// On Mondays that is Friday morning.
func sinceClause(flag string, now time.Time) (string, error) {
	if flag != "" {
		if !strings.HasSuffix(flag, "d") && !strings.HasSuffix(flag, "h") {
			return "", fmt.Errorf("--since must look like 1d or 12h")
		}
		return "-" + flag, nil
	}
	days := 1
	switch now.Weekday() {
	case time.Monday:
		days = 3
	case time.Sunday:
		days = 2
	}
	return fmt.Sprintf("startOfDay(-%d)", days), nil
}

// maxPerSection caps how many issues each standup section lists.
const maxPerSection = 50

type section struct {
	title  string
	jqls   []string // tried in order; later ones are fallbacks
	issues []*jira.Issue
}

func run(cmd *cobra.Command, _ []string) error {
	sinceFlag, _ := cmd.Flags().GetString("since")
	since, err := sinceClause(sinceFlag, time.Now())
	if err != nil {
		return err
	}
	sections := []*section{
		{title: "Yesterday", jqls: []string{
			fmt.Sprintf("status changed BY currentUser() AFTER %s ORDER BY updated DESC", since),
			fmt.Sprintf("worklogAuthor = currentUser() AND worklogDate >= %s ORDER BY updated DESC", since),
		}},
		{title: "Today", jqls: []string{
			`assignee = currentUser() AND statusCategory = "In Progress" ORDER BY priority DESC, updated DESC`,
		}},
		{title: "Blockers", jqls: []string{
			// "Flagged" only exists with Jira Software; fall back to the status.
			`assignee = currentUser() AND resolution = Unresolved AND (status = Blocked OR Flagged is not EMPTY)`,
			`assignee = currentUser() AND resolution = Unresolved AND status = Blocked`,
		}},
	}

	client := api.DefaultClient(viper.GetBool("debug"))
	s := cmdutil.Info("Collecting your standup...")
	for _, sec := range sections {
		seen := map[string]bool{}
		for i, jql := range sec.jqls {
			res, err := api.ProxySearch(client, jql, 0, maxPerSection)
			if err != nil {
				if sec.title == "Blockers" && i == 0 {
					continue // try the fallback query
				}
				s.Stop()
				return fmt.Errorf("%s: %w", sec.title, err)
			}
			for _, iss := range res.Issues {
				if !seen[iss.Key] {
					seen[iss.Key] = true
					sec.issues = append(sec.issues, iss)
				}
			}
			if sec.title == "Blockers" {
				break
			}
		}
	}
	s.Stop()

	links, _ := cmd.Flags().GetBool("links")
	var b strings.Builder
	render(&b, sections, viper.GetString("server"), links)
	out := b.String()

	if _, err := io.WriteString(cmd.OutOrStdout(), out); err != nil {
		return err
	}
	if c, _ := cmd.Flags().GetBool("copy"); c {
		if err := clipboard.WriteAll(out); err != nil {
			return fmt.Errorf("copy to clipboard: %w", err)
		}
		cmdutil.Success("Copied to clipboard")
	}
	return nil
}

func render(w io.Writer, sections []*section, server string, links bool) {
	for i, sec := range sections {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintf(w, "**%s**\n", sec.title)
		if len(sec.issues) == 0 {
			_, _ = fmt.Fprintln(w, "- nothing")
			continue
		}
		for _, iss := range sec.issues {
			key := iss.Key
			if links && server != "" {
				key = fmt.Sprintf("[%s](%s)", iss.Key, cmdutil.GenerateServerBrowseURL(server, iss.Key))
			}
			_, _ = fmt.Fprintf(w, "- %s %s — %s\n", key, iss.Fields.Summary, iss.Fields.Status.Name)
		}
	}
}
