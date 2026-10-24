package branch

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	jtbranch "github.com/aakara-dev/jt/internal/branch"
	"github.com/aakara-dev/jt/internal/cmdutil"
	"github.com/aakara-dev/jt/internal/gitutil"
	"github.com/aakara-dev/jt/pkg/jira"
)

const help = `Create (or switch to) a git branch named after an issue, e.g.
feat/DATA-12-add-retry-to-the-loader (bugs get fix/).

Without ISSUE-KEY you pick from your open issues. Commands that take an
issue key (view, move, assign, comment add, ...) use the key in the
current branch name when you leave it out.`

const examples = `$ jt branch DATA-12
$ jt branch                  # pick one of your open issues
$ jt branch DATA-12 --start   # also move it to In Progress
$ jt branch DATA-12 --print  # just print the name`

// NewCmdBranch is the branch command.
func NewCmdBranch() *cobra.Command {
	cmd := cobra.Command{
		Use:     "branch [ISSUE-KEY]",
		Short:   "Create a git branch for an issue",
		Long:    help,
		Example: examples,
		Aliases: []string{"br"},
		Args:    cobra.MaximumNArgs(1),
		RunE:    run,
	}
	cmd.Flags().BoolP("print", "n", false, "Print the branch name without touching git")
	cmd.Flags().String("base", "", "Start the new branch from this ref instead of HEAD")
	cmd.Flags().BoolP("start", "s", false, "Transition the issue to an in-progress status")
	return &cmd
}

func run(cmd *cobra.Command, args []string) error {
	client := api.DefaultClient(viper.GetBool("debug"))
	key := ""
	if len(args) == 1 {
		key = cmdutil.GetJiraIssueKey(viper.GetString("project.key"), args[0])
	} else {
		var err error
		if key, err = pickIssue(client); err != nil {
			return err
		}
	}

	iss, err := api.ProxyGetIssue(client, key)
	if err != nil {
		return err
	}
	name := jtbranch.Name(iss.Key, iss.Fields.Summary, iss.Fields.IssueType.Name)

	if p, _ := cmd.Flags().GetBool("print"); p {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), name)
		return err
	}

	base, _ := cmd.Flags().GetString("base")
	created, err := gitutil.Switch(name, base)
	if err != nil {
		return err
	}
	if created {
		cmdutil.Success("Created and switched to %s", name)
	} else {
		cmdutil.Success("Switched to existing %s", name)
	}

	if start, _ := cmd.Flags().GetBool("start"); start {
		return startProgress(client, iss)
	}
	return nil
}

const (
	pickLimit    = 30 // issues offered when no key is given
	pickPageSize = 15
)

// pickIssue lets the user choose one of their open issues.
func pickIssue(client *jira.Client) (string, error) {
	res, err := api.ProxySearch(client, "assignee = currentUser() AND resolution = Unresolved ORDER BY updated DESC", 0, pickLimit)
	if err != nil {
		return "", err
	}
	if len(res.Issues) == 0 {
		return "", fmt.Errorf("you have no open issues; pass an issue key")
	}
	options := make([]string, 0, len(res.Issues))
	for _, iss := range res.Issues {
		options = append(options, fmt.Sprintf("%s  %s  [%s]", iss.Key, iss.Fields.Summary, iss.Fields.Status.Name))
	}
	var choice string
	if err := survey.AskOne(&survey.Select{Message: "Branch for:", Options: options, PageSize: pickPageSize}, &choice); err != nil {
		return "", err
	}
	key, _, _ := strings.Cut(choice, " ")
	return key, nil
}

// startProgress moves the issue into the first in-progress status it can reach.
func startProgress(client *jira.Client, iss *jira.Issue) error {
	if iss.Fields.Status.Category.Key == "indeterminate" {
		return nil
	}
	ts, err := client.TransitionsV2(iss.Key)
	if err != nil {
		return err
	}
	for _, t := range ts {
		if t.To.Category.Key == "indeterminate" {
			if _, err := client.Transition(iss.Key, &jira.TransitionRequest{
				Transition: &jira.TransitionRequestData{ID: t.ID.String(), Name: t.Name},
			}); err != nil {
				return err
			}
			cmdutil.Success("%s → %s", iss.Key, t.To.Name)
			return nil
		}
	}
	cmdutil.Warn("No transition from %q to an in-progress status", iss.Fields.Status.Name)
	return nil
}
