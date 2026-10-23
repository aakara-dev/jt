package tui

import (
	"fmt"
	"strings"
)

// ListSpec describes an issue list view: a title and the JQL behind it.
type ListSpec struct {
	Name  string // command name, e.g. "mine"
	Title string
	JQL   string
}

// and joins non-empty JQL clauses.
func and(clauses ...string) string {
	var out []string
	for _, c := range clauses {
		if strings.TrimSpace(c) != "" {
			out = append(out, c)
		}
	}
	return strings.Join(out, " AND ")
}

func ordered(jql, order string) string {
	if strings.Contains(strings.ToUpper(jql), "ORDER BY") {
		return jql
	}
	return jql + " ORDER BY " + order
}

// BuiltinList returns the spec for a built-in list name, or false.
func BuiltinList(name string, s Session) (ListSpec, bool) {
	p := s.ProjectClause()
	switch name {
	case "issues", "is", "list":
		return ListSpec{"issues", "Issues · " + s.Project, ordered(and(p), "updated DESC")}, true
	case "mine", "my":
		return ListSpec{"mine", "Assigned to me", ordered(and("assignee = currentUser()", "resolution = Unresolved"), "updated DESC")}, true
	case "sprint", "sp":
		return ListSpec{"sprint", "Active sprint · " + s.Project, ordered(and(p, "sprint in openSprints()"), "status ASC, rank ASC")}, true
	case "backlog", "bl":
		return ListSpec{"backlog", "Backlog · " + s.Project, ordered(and(p, "sprint is EMPTY", "resolution = Unresolved", "issuetype not in subTaskIssueTypes()"), "rank ASC")}, true
	case "recent", "history":
		return ListSpec{"recent", "Recently viewed", "issuekey in issueHistory() ORDER BY lastViewed DESC"}, true
	case "watching", "watch":
		return ListSpec{"watching", "Watching", ordered("watcher = currentUser()", "updated DESC")}, true
	case "reported":
		return ListSpec{"reported", "Reported by me", ordered("reporter = currentUser()", "created DESC")}, true
	case "epics", "epic":
		return ListSpec{"epics", "Epics · " + s.Project, ordered(and(p, "issuetype = Epic"), "status ASC, updated DESC")}, true
	case "bugs":
		return ListSpec{"bugs", "Open bugs · " + s.Project, ordered(and(p, "issuetype = Bug", "resolution = Unresolved"), "priority DESC")}, true
	case "updated":
		return ListSpec{"updated", "Updated today · " + s.Project, ordered(and(p, "updated >= startOfDay()"), "updated DESC")}, true
	}
	return ListSpec{}, false
}

// BuiltinListNames are the list commands offered in completion and help.
var BuiltinListNames = []string{"issues", "mine", "sprint", "backlog", "recent", "watching", "reported", "epics", "bugs", "updated"}

// EpicChildren lists the issues of an epic (Jira DC "Epic Link").
func EpicChildren(key string) ListSpec {
	return ListSpec{"epic", "Epic " + key, fmt.Sprintf(`"Epic Link" = %s ORDER BY rank ASC`, key)}
}

// QueryList wraps raw JQL as a list.
func QueryList(title, jql string) ListSpec {
	return ListSpec{"query", title, jql}
}
