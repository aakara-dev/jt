package tui

import (
	"strconv"
	"strings"

	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/cmdcommon"
	"github.com/aakara-dev/jt/internal/cmdutil"
	"github.com/aakara-dev/jt/pkg/jira"
)

// Backend is the slice of the Jira API the TUI needs. It is an interface so
// the UI can be driven by a fake in tests.
type Backend interface {
	Search(jql string, from, limit uint) (*jira.SearchResult, error)
	Issue(key string) (*jira.Issue, error)
	FavouriteFilters() ([]*jira.SavedFilter, error)

	Transitions(key string) ([]*jira.Transition, error)
	Transition(key string, t *jira.Transition) error
	AssignableUsers(project, query string) ([]*jira.User, error)
	// Assign sets the assignee by username; jira.AssigneeNone unassigns.
	Assign(key, username string) error
	Comment(key, markdown string) error
	Worklog(key, timeSpent, comment string) error
	Edit(key string, req *jira.EditRequest) error
	Create(req *jira.CreateRequest) (*jira.CreateResponse, error)
	Watch(key, username string, watch bool) error
	LinkTypes() ([]*jira.IssueLinkType, error)
	Link(inward, outward, linkType string) error
	Delete(key string) error

	Board(boardID int) (*jira.BoardConfig, error)
	// Sprints lists sprints in the given comma separated states.
	Sprints(boardID int, states string) ([]*jira.Sprint, error)
	SprintIssues(sprintID int, jql string) (*jira.SearchResult, error)
	BoardIssues(boardID int, jql string) (*jira.SearchResult, error)
	MoveToSprint(sprintID int, keys []string) error
	MoveToBacklog(keys []string) error
	Rank(keys []string, before, after string) error
}

// jiraBackend talks to Jira Server/DC through the REST v2 and Agile APIs.
type jiraBackend struct{ c *jira.Client }

// NewJiraBackend wraps a configured Jira client.
func NewJiraBackend(c *jira.Client) Backend { return jiraBackend{c: c} }

func (b jiraBackend) Search(jql string, from, limit uint) (*jira.SearchResult, error) {
	return api.ProxySearch(b.c, jql, from, limit)
}

func (b jiraBackend) Issue(key string) (*jira.Issue, error) {
	return api.ProxyGetIssue(b.c, key)
}

func (b jiraBackend) FavouriteFilters() ([]*jira.SavedFilter, error) {
	return b.c.FavouriteFiltersV2()
}

func (b jiraBackend) Transitions(key string) ([]*jira.Transition, error) {
	return b.c.TransitionsV2(key)
}

func (b jiraBackend) Transition(key string, t *jira.Transition) error {
	_, err := b.c.Transition(key, &jira.TransitionRequest{
		Transition: &jira.TransitionRequestData{ID: t.ID.String(), Name: t.Name},
	})
	return err
}

func (b jiraBackend) AssignableUsers(project, query string) ([]*jira.User, error) {
	return b.c.UserSearchV2(&jira.UserSearchOptions{Project: project, Query: query, MaxResults: 20})
}

func (b jiraBackend) Assign(key, username string) error {
	return b.c.AssignIssueV2(key, username)
}

func (b jiraBackend) Comment(key, markdown string) error {
	return b.c.AddIssueComment(key, markdown, false)
}

func (b jiraBackend) Worklog(key, timeSpent, comment string) error {
	return b.c.AddIssueWorklog(key, "", timeSpent, comment, "")
}

func (b jiraBackend) Edit(key string, req *jira.EditRequest) error {
	return b.c.Edit(key, req)
}

// Create mirrors `jt issue create`: it fills in the installation-specific
// epic/sub-task fields and configured custom fields from the config.
func (b jiraBackend) Create(req *jira.CreateRequest) (*jira.CreateResponse, error) {
	req.EpicField = viper.GetString("epic.link")
	req.ForProjectType(viper.GetString("project.type"))
	req.ForInstallationType(viper.GetString("installation"))
	if fields, err := cmdcommon.GetConfiguredCustomFields(); err == nil {
		req.WithCustomFields(fields)
	}
	if handle := cmdutil.GetSubtaskHandle(req.IssueType, IssueTypes()); handle != "" {
		req.SubtaskField = handle
	}
	return api.ProxyCreate(b.c, req)
}

func (b jiraBackend) Watch(key, username string, watch bool) error {
	if watch {
		return b.c.WatchIssueV2(key, username)
	}
	return b.c.UnwatchIssueV2(key, username)
}

func (b jiraBackend) LinkTypes() ([]*jira.IssueLinkType, error) {
	return b.c.GetIssueLinkTypes()
}

func (b jiraBackend) Link(inward, outward, linkType string) error {
	return b.c.LinkIssue(inward, outward, linkType)
}

func (b jiraBackend) Delete(key string) error {
	return b.c.DeleteIssue(key, false)
}

func (b jiraBackend) Board(boardID int) (*jira.BoardConfig, error) {
	return b.c.BoardConfiguration(boardID)
}

func (b jiraBackend) Sprints(boardID int, states string) ([]*jira.Sprint, error) {
	res, err := b.c.Sprints(boardID, "state="+states, 0, 50)
	if err != nil {
		return nil, err
	}
	return res.Sprints, nil
}

func (b jiraBackend) SprintIssues(sprintID int, jql string) (*jira.SearchResult, error) {
	return b.c.SprintIssues(sprintID, jql, 0, 200)
}

func (b jiraBackend) BoardIssues(boardID int, jql string) (*jira.SearchResult, error) {
	return b.c.BoardIssues(boardID, jql, 0, 200)
}

func (b jiraBackend) MoveToSprint(sprintID int, keys []string) error {
	return b.c.SprintIssuesAdd(strconv.Itoa(sprintID), keys...)
}

func (b jiraBackend) MoveToBacklog(keys []string) error {
	return b.c.MoveToBacklog(keys...)
}

func (b jiraBackend) Rank(keys []string, before, after string) error {
	return b.c.RankIssues(keys, before, after)
}

// IssueTypes are the active project's issue types from the config.
func IssueTypes() []*jira.IssueType {
	var types []*jira.IssueType
	_ = viper.UnmarshalKey("issue.types", &types)
	return types
}

// issueTypeNames lists creatable (non sub-task) types, falling back to the
// Jira defaults when the config has none.
func issueTypeNames(subtasks bool) []string {
	var out []string
	for _, t := range IssueTypes() {
		if t.Subtask == subtasks {
			out = append(out, t.Name)
		}
	}
	if len(out) == 0 {
		if subtasks {
			return []string{jira.IssueTypeSubTask}
		}
		return []string{"Story", "Task", "Bug", "Epic"}
	}
	return out
}

// isDone reports whether a status is in the done category.
func isDone(s jira.IssueStatus) bool {
	return s.Category.Key == "done" || strings.EqualFold(s.Name, "done")
}
