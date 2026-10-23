// Package tui is jt's interactive terminal UI, built on Bubble Tea v2.
package tui

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/aakara-dev/jt/api"
	"github.com/aakara-dev/jt/internal/cmdutil"
	jtConfig "github.com/aakara-dev/jt/internal/config"
	"github.com/aakara-dev/jt/pkg/jira"
)

// Backend is the slice of the Jira API the TUI needs. It is an interface so
// the UI can be driven by a fake in tests.
type Backend interface {
	Search(jql string, from, limit uint) (*jira.SearchResult, error)
	Issue(key string) (*jira.Issue, error)
	FavouriteFilters() ([]*jira.SavedFilter, error)
}

// jiraBackend talks to Jira Server/DC through the REST v2 client.
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

// Session is the user-facing state that survives view changes: who we are,
// which server, and which saved project is active.
type Session struct {
	Server   string
	Login    string
	Name     string // saved project name (current_project)
	Project  string
	BoardID  int
	Projects []jtConfig.SavedProject
	Queries  map[string]string // saved_queries from config
	Theme    string
}

// LoadSession snapshots the current viper state (after the project overlay).
func LoadSession() Session {
	s := Session{
		Server:  strings.TrimRight(viper.GetString("server"), "/"),
		Login:   viper.GetString("login"),
		Name:    viper.GetString(jtConfig.KeyCurrentProject),
		Project: viper.GetString("project.key"),
		BoardID: viper.GetInt("board.id"),
		Queries: viper.GetStringMapString(jtConfig.KeySavedQueries),
		Theme:   viper.GetString("theme"),
	}
	if f, err := jtConfig.LoadCurrent(); err == nil {
		s.Projects = f.Projects()
	}
	return s
}

// SwitchProject persists name as the current project, re-applies it to
// viper and returns the refreshed session.
func SwitchProject(name string) (Session, error) {
	f, err := jtConfig.LoadCurrent()
	if err != nil {
		return Session{}, err
	}
	name, err = f.UseProject(name)
	if err != nil {
		return Session{}, err
	}
	if err := f.Save(); err != nil {
		return Session{}, err
	}
	if err := jtConfig.ApplyProject(viper.GetViper(), name); err != nil {
		return Session{}, err
	}
	return LoadSession(), nil
}

// SetTheme persists the theme choice.
func SetTheme(name string) error {
	f, err := jtConfig.LoadCurrent()
	if err != nil {
		return err
	}
	if err := f.Set(name, "theme"); err != nil {
		return err
	}
	viper.Set("theme", name)
	return f.Save()
}

// SaveQuery stores a named JQL query under saved_queries.
func SaveQuery(name, jql string) error {
	f, err := jtConfig.LoadCurrent()
	if err != nil {
		return err
	}
	if err := f.Set(jql, jtConfig.KeySavedQueries, name); err != nil {
		return err
	}
	return f.Save()
}

// BrowseURL is the web URL of an issue.
func (s Session) BrowseURL(key string) string {
	return cmdutil.GenerateServerBrowseURL(s.Server, key)
}

// ExpandJQL substitutes {project}/{me} placeholders in saved queries.
func (s Session) ExpandJQL(jql string) string {
	return strings.NewReplacer("{project}", s.Project, "{{project}}", s.Project, "{me}", s.Login).Replace(jql)
}

// ProjectClause scopes a query to the active project, if any.
func (s Session) ProjectClause() string {
	if s.Project == "" {
		return ""
	}
	return fmt.Sprintf("project = %q", s.Project)
}
