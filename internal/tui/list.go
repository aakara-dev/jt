package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/aakara-dev/jt/pkg/jira"
)

const pageSize = 100

// issuesLoadedMsg carries one page of search results for a list.
type issuesLoadedMsg struct {
	id     int
	from   uint
	result *jira.SearchResult
	err    error
}

var listSeq int

// listView is a scrollable, filterable table of issues backed by JQL.
type listView struct {
	id      int
	spec    ListSpec
	backend Backend
	st      *styles

	issues  []*jira.Issue
	total   int
	loading bool
	err     error

	visible []int // indices into issues after the inline filter
	cursor  int
	offset  int
	height  int

	filtering bool
	filter    textinput.Model
	pendingG  bool
	preview   bool
}

func newListView(spec ListSpec, b Backend, st *styles) *listView {
	listSeq++
	in := textinput.New()
	in.Prompt = "/"
	return &listView{id: listSeq, spec: spec, backend: b, st: st, filter: in, loading: true, preview: true}
}

func (v *listView) Init() tea.Cmd { return v.fetch(0) }

func (v *listView) fetch(from uint) tea.Cmd {
	v.loading = true
	id, jql, b := v.id, v.spec.JQL, v.backend
	return func() tea.Msg {
		res, err := b.Search(jql, from, pageSize)
		return issuesLoadedMsg{id: id, from: from, result: res, err: err}
	}
}

func (v *listView) Title() string { return v.spec.Title }

func (v *listView) Capturing() bool { return v.filtering }

func (v *listView) Selected() *jira.Issue {
	if v.cursor < 0 || v.cursor >= len(v.visible) {
		return nil
	}
	return v.issues[v.visible[v.cursor]]
}

func (v *listView) Hints() []Hint {
	return []Hint{
		{"/", "filter"},
		{"p", "toggle preview"},
		{"enter", "details"},
		{"ctrl+r", "reload"},
	}
}

func (v *listView) applyFilter() {
	q := v.filter.Value()
	v.visible = v.visible[:0]
	if q == "" {
		for i := range v.issues {
			v.visible = append(v.visible, i)
		}
	} else {
		src := make([]string, len(v.issues))
		for i, iss := range v.issues {
			f := iss.Fields
			src[i] = strings.Join([]string{iss.Key, f.Summary, f.Status.Name, f.Assignee.Name, strings.Join(f.Labels, " ")}, " ")
		}
		for _, m := range fuzzy.Find(q, src) {
			v.visible = append(v.visible, m.Index)
		}
	}
	v.clamp()
}

func (v *listView) clamp() {
	if v.cursor >= len(v.visible) {
		v.cursor = len(v.visible) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
}

func (v *listView) move(delta int) tea.Cmd {
	v.cursor += delta
	v.clamp()
	// Infinite scroll: fetch the next page when nearing the end.
	if !v.loading && v.filter.Value() == "" && len(v.issues) < v.total && v.cursor >= len(v.visible)-5 {
		return v.fetch(uint(len(v.issues)))
	}
	return nil
}

func (v *listView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case issuesLoadedMsg:
		if msg.id != v.id {
			return v, nil
		}
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
			return v, toastErr(msg.err)
		}
		v.err = nil
		if msg.from == 0 {
			v.issues = msg.result.Issues
		} else {
			v.issues = append(v.issues, msg.result.Issues...)
		}
		v.total = max(msg.result.Total, len(v.issues))
		v.applyFilter()
		return v, nil

	case tea.KeyPressMsg:
		if v.filtering {
			return v.updateFilter(msg)
		}
		key := msg.String()
		if v.pendingG {
			v.pendingG = false
			if key == "g" {
				v.cursor = 0
				return v, nil
			}
		}
		half := max(v.height/2, 1)
		switch key {
		case "j", "down":
			return v, v.move(1)
		case "k", "up":
			return v, v.move(-1)
		case "g":
			v.pendingG = true
		case "G", "end":
			return v, v.move(len(v.visible))
		case "home":
			v.cursor = 0
		case "ctrl+d", "pgdown":
			return v, v.move(half)
		case "ctrl+u", "pgup":
			return v, v.move(-half)
		case "/":
			v.filtering = true
			return v, v.filter.Focus()
		case "p":
			v.preview = !v.preview
		case "ctrl+r":
			v.cursor = 0
			return v, v.fetch(0)
		case "enter", "l", "right":
			if iss := v.Selected(); iss != nil {
				return v, push(newDetailView(iss.Key, v.backend, v.st))
			}
		}
	}
	return v, nil
}

func (v *listView) updateFilter(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "esc":
		v.filtering = false
		v.filter.SetValue("")
		v.filter.Blur()
		v.applyFilter()
		return v, nil
	case "enter":
		v.filtering = false
		v.filter.Blur()
		return v, nil
	case "down", "ctrl+n", "ctrl+j":
		return v, v.move(1)
	case "up", "ctrl+p", "ctrl+k":
		return v, v.move(-1)
	}
	var cmd tea.Cmd
	before := v.filter.Value()
	v.filter, cmd = v.filter.Update(msg)
	if v.filter.Value() != before {
		v.cursor = 0
		v.applyFilter()
	}
	return v, cmd
}

func (v *listView) Render(width, height int) string {
	st := *v.st
	header := v.renderHeader(width)
	bodyH := height - lipgloss.Height(header)
	if v.filtering || v.filter.Value() != "" {
		bodyH--
	}
	v.height = bodyH

	listW := width
	showPreview := v.preview && width >= 110
	if showPreview {
		listW = width * 3 / 5
	}

	var body string
	switch {
	case v.loading && len(v.issues) == 0:
		body = st.Muted.Render("  loading…")
	case v.err != nil && len(v.issues) == 0:
		body = st.Error.Render("  " + v.err.Error())
	case len(v.visible) == 0:
		body = st.Muted.Render("  no issues")
	default:
		body = v.renderRows(listW, bodyH)
	}
	body = lipgloss.NewStyle().Width(listW).Height(bodyH).MaxHeight(bodyH).Render(body)

	if showPreview {
		pv := renderPreview(st, v.Selected(), width-listW-1, bodyH)
		sep := st.Muted.Render(strings.TrimRight(strings.Repeat("│\n", bodyH), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, sep, pv)
	}

	out := header + "\n" + body
	if v.filtering || v.filter.Value() != "" {
		out += "\n" + v.filter.View()
	}
	return out
}

func (v *listView) renderHeader(width int) string {
	st := *v.st
	count := fmt.Sprintf("%d", len(v.visible))
	if v.filter.Value() != "" {
		count = fmt.Sprintf("%d/%d", len(v.visible), len(v.issues))
	}
	if v.total > len(v.issues) {
		count += fmt.Sprintf(" of %d", v.total)
	}
	title := st.Title.Render(v.spec.Title) + " " + st.Muted.Render("("+count+")")
	if v.loading {
		title += " " + st.Warn.Render("⟳")
	}
	jql := st.Muted.Italic(true).Render(fit(v.spec.JQL, max(width-lipgloss.Width(title)-3, 0)))
	return " " + title + "  " + jql
}

// column widths for the issue table.
const (
	colKey      = 12
	colStatus   = 16
	colAssignee = 16
	colUpdated  = 5
)

func (v *listView) renderRows(width, height int) string {
	st := *v.st
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+height {
		v.offset = v.cursor - height + 1
	}
	summaryW := max(width-(2+2+colKey+2+colStatus+colAssignee+colUpdated+6), 10)
	showAssignee := width >= 80
	if !showAssignee {
		summaryW += colAssignee
	}

	var rows []string
	for i := v.offset; i < len(v.visible) && i < v.offset+height; i++ {
		iss := v.issues[v.visible[i]]
		f := iss.Fields
		cells := []string{
			st.typeIcon(f.IssueType),
			st.Accent.Render(fit(iss.Key, colKey)),
			st.priorityIcon(f.Priority.Name),
			fit(f.Summary, summaryW),
			fit(st.statusBadge(f.Status), colStatus),
		}
		if showAssignee {
			cells = append(cells, st.Subtle.Render(fit(orDash(f.Assignee.Name), colAssignee)))
		}
		cells = append(cells, st.Muted.Render(fit(ago(f.Updated), colUpdated)))
		line := strings.Join(cells, " ")
		if i == v.cursor {
			line = st.RowSelected.Width(width).Render("▌" + ansiStrip(line))
		} else {
			line = " " + line
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

// renderPreview shows the key facts of an issue next to the list.
func renderPreview(st styles, iss *jira.Issue, width, height int) string {
	box := lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).PaddingLeft(1)
	if iss == nil {
		return box.Render("")
	}
	f := iss.Fields
	inner := width - 2
	kv := func(k, val string) string {
		return st.Muted.Render(fit(k, 10)) + st.Value.Render(fit(val, inner-10))
	}
	lines := []string{
		st.typeIcon(f.IssueType) + " " + st.Accent.Bold(true).Render(iss.Key) + "  " + st.statusBadge(f.Status),
		lipgloss.NewStyle().Width(inner).Bold(true).Foreground(st.Palette.Text).Render(f.Summary),
		"",
		kv("Type", f.IssueType.Name),
		kv("Priority", orDash(f.Priority.Name)),
		kv("Assignee", orDash(f.Assignee.Name)),
		kv("Reporter", orDash(f.Reporter.Name)),
		kv("Labels", orDash(strings.Join(f.Labels, ", "))),
		kv("Updated", agoLabel(f.Updated)),
	}
	if desc := plainDescription(iss); desc != "" {
		lines = append(lines, "", st.Muted.Render(strings.Repeat("─", inner)))
		lines = append(lines, strings.TrimRight(cachedMarkdown(iss.Key, desc, inner+2), "\n"))
	}
	return box.Render(strings.Join(lines, "\n"))
}

func plainDescription(iss *jira.Issue) string {
	if s, ok := iss.Fields.Description.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "–"
	}
	return s
}

// ago formats a Jira timestamp as a compact relative age (3m, 5h, 2d, 7w).
func ago(ts string) string {
	t, err := time.Parse(jira.RFC3339, ts)
	if err != nil {
		return ""
	}
	return humanAge(time.Since(t))
}

// agoLabel is ago() with a suffix, or a dash for missing timestamps.
func agoLabel(ts string) string {
	if a := ago(ts); a != "" {
		return a + " ago"
	}
	return "–"
}

func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/24/7))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/24/365))
	}
}
