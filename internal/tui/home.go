package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aakara-dev/jt/pkg/jira"
)

// homeSection is one block on the dashboard.
type homeSection struct {
	title   string
	jql     string
	limit   uint
	issues  []*jira.Issue
	total   int
	loading bool
	err     error
}

type homeLoadedMsg struct {
	id      int
	section int
	res     *jira.SearchResult
	err     error
}

var homeSeq int

// homeView is the "my work" dashboard: what I'm doing, what's next, what
// changed around me. j/k move across all sections, enter opens an issue and
// O opens the current section as a full list.
type homeView struct {
	id       int
	backend  Backend
	st       *styles
	sections []*homeSection
	cursor   int // index into the flattened rows
}

func homeSections(s Session) []*homeSection {
	p := s.ProjectClause()
	return []*homeSection{
		{title: "In progress", jql: "assignee = currentUser() AND statusCategory = \"In Progress\" ORDER BY updated DESC", limit: 8},
		{title: "Up next", jql: "assignee = currentUser() AND statusCategory = \"To Do\" AND resolution = Unresolved ORDER BY priority DESC, updated DESC", limit: 8},
		{title: "Due this week", jql: "assignee = currentUser() AND resolution = Unresolved AND due <= endOfWeek() ORDER BY due ASC", limit: 5},
		{title: "Watching · updated in the last 2 days", jql: "watcher = currentUser() AND assignee != currentUser() AND updated >= -2d ORDER BY updated DESC", limit: 6},
		{title: "Reported by me · open", jql: and(p, "reporter = currentUser()", "resolution = Unresolved") + " ORDER BY updated DESC", limit: 5},
	}
}

func newHomeView(s Session, b Backend, st *styles) *homeView {
	homeSeq++
	return &homeView{id: homeSeq, backend: b, st: st, sections: homeSections(s)}
}

func (v *homeView) Init() tea.Cmd { return v.load() }

func (v *homeView) load() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(v.sections))
	for i, sec := range v.sections {
		sec.loading = true
		id, idx, jql, limit, b := v.id, i, sec.jql, sec.limit, v.backend
		cmds = append(cmds, func() tea.Msg {
			res, err := b.Search(jql, 0, limit)
			return homeLoadedMsg{id: id, section: idx, res: res, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (v *homeView) Title() string         { return "My work" }
func (v *homeView) Capturing() bool       { return false }
func (v *homeView) Marked() []*jira.Issue { return nil }

func (v *homeView) Hints() []Hint {
	return []Hint{
		{"j/k", "move across sections"},
		{"enter", "details"},
		{"]  [", "next / previous section"},
		{"O", "open the section as a full list"},
		{"ctrl+r", "reload"},
	}
}

// rows flattens sections into (section, issue) pairs.
func (v *homeView) rows() [][2]int {
	var out [][2]int
	for si, sec := range v.sections {
		for ii := range sec.issues {
			out = append(out, [2]int{si, ii})
		}
	}
	return out
}

func (v *homeView) Selected() *jira.Issue {
	rows := v.rows()
	if v.cursor < 0 || v.cursor >= len(rows) {
		return nil
	}
	r := rows[v.cursor]
	return v.sections[r[0]].issues[r[1]]
}

func (v *homeView) currentSection() int {
	rows := v.rows()
	if v.cursor < len(rows) {
		return rows[v.cursor][0]
	}
	return 0
}

func (v *homeView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case homeLoadedMsg:
		if msg.id != v.id {
			return v, nil
		}
		sec := v.sections[msg.section]
		sec.loading, sec.err = false, msg.err
		if msg.err == nil {
			sec.issues, sec.total = msg.res.Issues, max(msg.res.Total, len(msg.res.Issues))
		}
		v.cursor = max(min(v.cursor, len(v.rows())-1), 0)
		return v, nil
	case issuesChangedMsg:
		return v, v.load()
	case tea.KeyPressMsg:
		n := len(v.rows())
		switch msg.String() {
		case "j", "down":
			v.cursor = min(v.cursor+1, max(n-1, 0))
		case "k", "up":
			v.cursor = max(v.cursor-1, 0)
		case "g", "home":
			v.cursor = 0
		case "G", "end":
			v.cursor = max(n-1, 0)
		case "]", "[":
			v.jumpSection(msg.String() == "]")
		case "ctrl+r":
			return v, v.load()
		case "O":
			sec := v.sections[v.currentSection()]
			return v, push(newListView(QueryList(sec.title, sec.jql), v.backend, v.st))
		case "enter", "l", "right":
			if iss := v.Selected(); iss != nil {
				return v, push(newDetailView(iss.Key, v.backend, v.st))
			}
		}
	}
	return v, nil
}

// jumpSection moves the cursor to the first row of the next/previous
// non-empty section.
func (v *homeView) jumpSection(forward bool) {
	rows := v.rows()
	if len(rows) == 0 {
		return
	}
	cur := rows[v.cursor][0]
	if forward {
		for i := v.cursor; i < len(rows); i++ {
			if rows[i][0] != cur {
				v.cursor = i
				return
			}
		}
		return
	}
	// Back to the start of this section, or of the previous one.
	start := v.cursor
	for start > 0 && rows[start-1][0] == cur {
		start--
	}
	if start != v.cursor {
		v.cursor = start
		return
	}
	if start > 0 {
		prev := rows[start-1][0]
		for start > 0 && rows[start-1][0] == prev {
			start--
		}
		v.cursor = start
	}
}

func (v *homeView) Render(width, height int) string {
	st := *v.st
	summaryW := max(width-(4+colKey+colStatus+colUpdated+8), 10)

	var lines []string
	selLine, row := 0, 0
	for _, sec := range v.sections {
		count := ""
		switch {
		case sec.loading:
			count = st.Warn.Render("⟳")
		case sec.err != nil:
			count = st.Error.Render("✖ " + sec.err.Error())
		case sec.total > len(sec.issues):
			count = st.Muted.Render(fmt.Sprintf("%d of %d", len(sec.issues), sec.total))
		default:
			count = st.Muted.Render(fmt.Sprint(len(sec.issues)))
		}
		head := " " + st.Accent2.Bold(true).Render(sec.title) + " " + count
		lines = append(lines, head+" "+st.Muted.Render(strings.Repeat("─", max(width-lipgloss.Width(head)-2, 0))))
		if len(sec.issues) == 0 && !sec.loading && sec.err == nil {
			lines = append(lines, st.Muted.Render("   nothing here"))
		}
		for _, iss := range sec.issues {
			f := iss.Fields
			line := strings.Join([]string{
				st.typeIcon(f.IssueType),
				st.Accent.Render(fit(iss.Key, colKey)),
				st.priorityIcon(f.Priority.Name),
				fit(f.Summary, summaryW),
				fit(st.statusBadge(f.Status), colStatus),
				st.Muted.Render(fit(ago(f.Updated), colUpdated)),
			}, " ")
			// Compare by row, not issue: one issue can sit in several sections.
			if row == v.cursor {
				selLine = len(lines)
				line = st.RowSelected.Width(width).Render(" ▌" + ansiStrip(line))
			} else {
				line = "  " + line
			}
			lines = append(lines, line)
			row++
		}
		lines = append(lines, "")
	}

	// Scroll so the selected row stays visible.
	start := 0
	if selLine >= height {
		start = selLine - height + 2
	}
	end := min(start+height, len(lines))
	return strings.Join(lines[start:end], "\n")
}
