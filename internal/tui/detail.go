package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"

	"github.com/aakara-dev/jt/pkg/jira"
	"github.com/aakara-dev/jt/pkg/md"
)

type issueLoadedMsg struct {
	key string
	iss *jira.Issue
	err error
}

// detailView shows one issue: metadata, description, subtasks, links and
// comments in a scrollable document.
type detailView struct {
	key     string
	backend Backend
	st      *styles

	iss      *jira.Issue
	err      error
	vp       viewport.Model
	width    int
	pendingG bool
}

func newDetailView(key string, b Backend, st *styles) *detailView {
	return &detailView{key: key, backend: b, st: st, vp: viewport.New()}
}

func (v *detailView) Init() tea.Cmd { return v.fetch() }

func (v *detailView) fetch() tea.Cmd {
	key, b := v.key, v.backend
	return func() tea.Msg {
		iss, err := b.Issue(key)
		return issueLoadedMsg{key: key, iss: iss, err: err}
	}
}

func (v *detailView) Title() string { return v.key }

func (v *detailView) Capturing() bool { return false }

func (v *detailView) Selected() *jira.Issue {
	if v.iss != nil {
		return v.iss
	}
	return &jira.Issue{Key: v.key}
}

func (v *detailView) Hints() []Hint {
	return []Hint{
		{"j/k", "scroll"},
		{"L", "linked issues & subtasks"},
		{"ctrl+r", "reload"},
	}
}

func (v *detailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case issueLoadedMsg:
		if msg.key != v.key {
			return v, nil
		}
		if msg.err != nil {
			v.err = msg.err
			return v, toastErr(msg.err)
		}
		v.iss, v.err = msg.iss, nil
		v.width = 0 // force re-render
		iss := msg.iss
		return v, func() tea.Msg { return viewedIssueMsg{iss} }

	case tea.KeyPressMsg:
		key := msg.String()
		if v.pendingG {
			v.pendingG = false
			if key == "g" {
				v.vp.GotoTop()
				return v, nil
			}
		}
		switch key {
		case "g":
			v.pendingG = true
			return v, nil
		case "G":
			v.vp.GotoBottom()
			return v, nil
		case "h", "left":
			return v, pop
		case "ctrl+r":
			return v, v.fetch()
		case "L":
			return v, v.relatedPicker()
		}
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return v, cmd
}

func (v *detailView) relatedPicker() tea.Cmd {
	if v.iss == nil {
		return nil
	}
	var items []pickItem
	f := v.iss.Fields
	if f.Parent != nil && f.Parent.Key != "" {
		items = append(items, pickItem{Label: f.Parent.Key, Detail: "parent", Value: f.Parent.Key})
	}
	for _, s := range f.Subtasks {
		items = append(items, pickItem{Label: s.Key, Detail: "subtask · " + s.Fields.Summary, Value: s.Key})
	}
	for _, l := range f.IssueLinks {
		if l.InwardIssue != nil {
			items = append(items, pickItem{Label: l.InwardIssue.Key, Detail: l.LinkType.Inward + " · " + l.InwardIssue.Fields.Summary, Value: l.InwardIssue.Key})
		}
		if l.OutwardIssue != nil {
			items = append(items, pickItem{Label: l.OutwardIssue.Key, Detail: l.LinkType.Outward + " · " + l.OutwardIssue.Fields.Summary, Value: l.OutwardIssue.Key})
		}
	}
	if len(items) == 0 {
		return toast("no linked issues")
	}
	b, st := v.backend, v.st
	return openPopup(newPicker("related", items, func(it pickItem) tea.Cmd {
		return push(newDetailView(it.Value, b, st))
	}))
}

func (v *detailView) Render(width, height int) string {
	st := *v.st
	if v.iss == nil {
		if v.err != nil {
			return st.Error.Render("  " + v.err.Error())
		}
		return st.Muted.Render("  loading " + v.key + "…")
	}
	if width != v.width {
		v.width = width
		v.vp.SetWidth(width)
		v.vp.SetContent(v.document(width))
	}
	v.vp.SetHeight(height)
	return v.vp.View()
}

func (v *detailView) document(width int) string {
	st := *v.st
	iss := v.iss
	f := iss.Fields
	inner := min(width-2, 120)

	var b strings.Builder
	b.WriteString(" " + st.typeIcon(f.IssueType) + " " + st.Accent.Bold(true).Render(iss.Key) + "  " + st.statusBadge(f.Status) + "  " + st.priorityIcon(f.Priority.Name) + " " + st.Muted.Render(orDash(f.Priority.Name)))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(st.Palette.Text).Width(inner).PaddingLeft(1).Render(f.Summary))
	b.WriteString("\n\n")

	meta := [][2]string{
		{"Type", f.IssueType.Name},
		{"Assignee", orDash(f.Assignee.Name)},
		{"Reporter", orDash(f.Reporter.Name)},
		{"Labels", orDash(strings.Join(f.Labels, ", "))},
		{"Components", orDash(joinNames(f.Components))},
		{"Fix versions", orDash(joinNames(f.FixVersions))},
		{"Created", agoLabel(f.Created)},
		{"Updated", agoLabel(f.Updated)},
		{"Watchers", fmt.Sprintf("%d", f.Watches.WatchCount)},
	}
	if f.Parent != nil && f.Parent.Key != "" {
		meta = append([][2]string{{"Parent", f.Parent.Key}}, meta...)
	}
	for _, kv := range meta {
		b.WriteString(" " + st.Muted.Render(fit(kv[0], 14)) + st.Value.Render(kv[1]) + "\n")
	}

	section := func(title string) {
		b.WriteString("\n " + st.Accent2.Bold(true).Render(title) + " " + st.Muted.Render(strings.Repeat("─", max(inner-lipgloss.Width(title)-2, 0))) + "\n")
	}

	if desc := plainDescription(iss); desc != "" {
		section("Description")
		b.WriteString(cachedMarkdown(iss.Key, desc, inner))
	}

	if len(f.Subtasks) > 0 {
		section(fmt.Sprintf("Subtasks (%d)", len(f.Subtasks)))
		for _, s := range f.Subtasks {
			b.WriteString(" " + st.Accent.Render(fit(s.Key, colKey)) + " " + fit(st.statusBadge(s.Fields.Status), colStatus) + " " + s.Fields.Summary + "\n")
		}
	}

	if len(f.IssueLinks) > 0 {
		section(fmt.Sprintf("Links (%d)", len(f.IssueLinks)))
		for _, l := range f.IssueLinks {
			rel, other := l.LinkType.Outward, l.OutwardIssue
			if other == nil {
				rel, other = l.LinkType.Inward, l.InwardIssue
			}
			if other == nil {
				continue
			}
			b.WriteString(" " + st.Muted.Render(fit(rel, 16)) + st.Accent.Render(fit(other.Key, colKey)) + " " + fit(st.statusBadge(other.Fields.Status), colStatus) + " " + other.Fields.Summary + "\n")
		}
	}

	comments := f.Comment.Comments
	if len(comments) > 0 {
		section(fmt.Sprintf("Comments (%d)", f.Comment.Total))
		for i := len(comments) - 1; i >= 0; i-- {
			c := comments[i]
			body, _ := c.Body.(string)
			b.WriteString(" " + st.Key.Render(c.Author.DisplayName) + " " + st.Muted.Render("· "+agoLabel(c.Created)) + "\n")
			b.WriteString(renderMarkdown(md.FromJiraMD(body), inner))
		}
	}
	return b.String()
}

func joinNames(items []struct {
	Name string `json:"name"`
},
) string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return strings.Join(names, ", ")
}

// mdCache memoises rendered descriptions; Render runs on every frame and
// glamour is too slow to call that often.
var mdCache = map[string]string{}

// cachedMarkdown renders a Jira wiki-markup description at width, cached by
// issue key, width and content.
func cachedMarkdown(key, jiraMarkup string, width int) string {
	k := fmt.Sprintf("%s|%d|%d", key, width, len(jiraMarkup))
	if out, ok := mdCache[k]; ok {
		return out
	}
	if len(mdCache) > 500 {
		mdCache = map[string]string{}
	}
	out := renderMarkdown(md.FromJiraMD(jiraMarkup), width)
	mdCache[k] = out
	return out
}

// renderMarkdown renders markdown for the terminal, falling back to plain
// text if glamour fails.
func renderMarkdown(src string, width int) string {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width-4))
	if err != nil {
		return src + "\n"
	}
	out, err := r.Render(src)
	if err != nil {
		return src + "\n"
	}
	return out
}
