package tui

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aakara-dev/jt/pkg/jira"
)

// --- sprint stats -------------------------------------------------------------

// statsPopup summarises the board: issues per column, per assignee and per
// type, with completion bars.
type statsPopup struct {
	title   string
	columns []*boardColumn
}

func (p statsPopup) Update(msg tea.Msg) (Popup, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc", "q", "S", "enter", "ctrl+c":
			return p, closePopup
		}
	}
	return p, nil
}

type tally struct {
	name        string
	done, total int
}

func countBy(columns []*boardColumn, key func(*jira.Issue) string) []tally {
	m := map[string]*tally{}
	last := len(columns) - 1
	for ci, c := range columns {
		for _, iss := range c.issues {
			k := key(iss)
			if m[k] == nil {
				m[k] = &tally{name: k}
			}
			m[k].total++
			if ci == last || isDone(iss.Fields.Status) {
				m[k].done++
			}
		}
	}
	out := make([]tally, 0, len(m))
	for _, t := range m {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].total != out[j].total {
			return out[i].total > out[j].total
		}
		return out[i].name < out[j].name
	})
	return out
}

func (p statsPopup) Render(st styles, width, _ int) string {
	barW := 20
	section := func(title string, rows []tally) string {
		lines := []string{st.Accent2.Bold(true).Render(title)}
		nameW := 0
		for _, r := range rows {
			nameW = max(nameW, lipgloss.Width(r.name))
		}
		nameW = min(nameW, 22)
		for _, r := range rows {
			lines = append(lines, fit(r.name, nameW)+"  "+progressBar(st, r.done, r.total, barW)+st.Muted.Render(fmt.Sprintf(" %d/%d", r.done, r.total)))
		}
		return strings.Join(lines, "\n")
	}

	var cols []tally
	total, done := 0, 0
	for ci, c := range p.columns {
		t := tally{name: c.name, total: len(c.issues)}
		if ci == len(p.columns)-1 {
			t.done = t.total
		}
		total += t.total
		done += t.done
		cols = append(cols, t)
	}
	byAssignee := countBy(p.columns, func(iss *jira.Issue) string { return orDash(iss.Fields.Assignee.Name) })
	byType := countBy(p.columns, func(iss *jira.Issue) string { return iss.Fields.IssueType.Name })

	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	head := st.Title.Render(fmt.Sprintf("%d%% done", pct)) + st.Muted.Render(fmt.Sprintf("  %d of %d issues", done, total))
	left := section("By column", cols) + "\n\n" + section("By type", byType)
	right := section("By assignee", byAssignee)
	body := head + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right)
	w := min(lipgloss.Width(body)+st.Popup.GetHorizontalFrameSize(), width-2)
	return frame(st, "stats · "+p.title, body, "esc close", w)
}

// --- watched-issue notifications ------------------------------------------------

type (
	watchTickMsg   struct{}
	watchResultMsg struct {
		issues []*jira.Issue
		err    error
	}
)

// watchJQL finds recently updated issues the user watches or is assigned.
func watchJQL(interval time.Duration) string {
	mins := max(int(interval.Minutes())*2, 2) // overlap ticks so nothing slips through
	return fmt.Sprintf("(watcher = currentUser() OR assignee = currentUser()) AND updated >= -%dm ORDER BY updated DESC", mins)
}

func (a *App) scheduleWatch() tea.Cmd {
	if a.sess.WatchInterval <= 0 {
		return nil
	}
	return tea.Tick(a.sess.WatchInterval, func(time.Time) tea.Msg { return watchTickMsg{} })
}

func (a *App) pollWatched() tea.Cmd {
	b, jql := a.backend, watchJQL(a.sess.WatchInterval)
	return func() tea.Msg {
		res, err := b.Search(jql, 0, 20)
		if err != nil {
			return watchResultMsg{err: err}
		}
		return watchResultMsg{issues: res.Issues}
	}
}

// onWatchResult compares updated timestamps with what was seen before. The
// first poll only records a baseline, so starting jt never floods toasts.
func (a *App) onWatchResult(msg watchResultMsg) tea.Cmd {
	next := a.scheduleWatch()
	if msg.err != nil {
		return next // transient; try again next tick
	}
	var fresh []*jira.Issue
	for _, iss := range msg.issues {
		prev, seen := a.watchSeen[iss.Key]
		if a.watchPrimed && (!seen || prev != iss.Fields.Updated) {
			fresh = append(fresh, iss)
		}
		a.watchSeen[iss.Key] = iss.Fields.Updated
	}
	a.watchPrimed = true
	if len(fresh) == 0 {
		return next
	}
	text := fmt.Sprintf("%s updated · %s", fresh[0].Key, fresh[0].Fields.Summary)
	if len(fresh) > 1 {
		text = fmt.Sprintf("%d watched issues updated (%s, …)", len(fresh), fresh[0].Key)
	}
	cmds := []tea.Cmd{next, toast(text), func() tea.Msg { return changed(keys(fresh)...) }}
	if a.sess.Bell {
		cmds = append(cmds, bell)
	}
	return tea.Batch(cmds...)
}

// bell rings the terminal bell.
func bell() tea.Msg {
	_, _ = os.Stdout.WriteString("\a")
	return nil
}

// --- export ---------------------------------------------------------------------

// exportable is implemented by views that can export their issues.
type exportable interface {
	exportIssues() (name string, issues []*jira.Issue)
}

func (v *listView) exportIssues() (string, []*jira.Issue) {
	out := make([]*jira.Issue, 0, len(v.visible))
	for _, i := range v.visible {
		out = append(out, v.issues[i])
	}
	return v.spec.Name, out
}

func (v *boardView) exportIssues() (string, []*jira.Issue) {
	var out []*jira.Issue
	for _, c := range v.columns {
		out = append(out, c.issues...)
	}
	return "board", out
}

func (v *homeView) exportIssues() (string, []*jira.Issue) {
	var out []*jira.Issue
	for _, s := range v.sections {
		out = append(out, s.issues...)
	}
	return "home", out
}

var exportHeader = []string{"Key", "Type", "Status", "Priority", "Assignee", "Summary", "Updated"}

func exportRow(iss *jira.Issue) []string {
	f := iss.Fields
	return []string{iss.Key, f.IssueType.Name, f.Status.Name, f.Priority.Name, f.Assignee.Name, f.Summary, f.Updated}
}

// exportView writes the top view's issues as CSV or a markdown table.
// Format comes from arg ("csv", "md") or the path's extension.
func (a *App) exportView(arg string) tea.Cmd {
	ev, ok := a.top().(exportable)
	if !ok {
		return toast("nothing to export in this view")
	}
	name, issues := ev.exportIssues()
	format, path, _ := strings.Cut(strings.TrimSpace(arg), " ")
	path = strings.TrimSpace(path)
	switch {
	case format == "":
		format = "csv"
	case strings.Contains(format, "."):
		path, format = format, strings.TrimPrefix(filepath.Ext(format), ".")
	}
	if format == "markdown" {
		format = "md"
	}
	if format != "csv" && format != "md" {
		return errorf("export format must be csv or md, got %q", format)
	}
	if path == "" {
		path = fmt.Sprintf("jt-%s-%s.%s", name, time.Now().Format("20060102-1504"), format)
	}
	var err error
	if format == "csv" {
		err = writeCSV(path, issues)
	} else {
		err = writeMarkdown(path, issues)
	}
	if err != nil {
		return toastErr(err)
	}
	abs, _ := filepath.Abs(path)
	return toast(fmt.Sprintf("exported %d issues to %s", len(issues), abs))
}

func writeCSV(path string, issues []*jira.Issue) error {
	f, err := os.Create(path) //nolint:gosec // user-chosen export path
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write(exportHeader)
	for _, iss := range issues {
		_ = w.Write(exportRow(iss))
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeMarkdown(path string, issues []*jira.Issue) error {
	esc := strings.NewReplacer("|", `\|`, "\n", " ")
	var b strings.Builder
	b.WriteString("| " + strings.Join(exportHeader, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" --- |", len(exportHeader)) + "\n")
	for _, iss := range issues {
		row := exportRow(iss)
		for i := range row {
			row[i] = esc.Replace(row[i])
		}
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644) //nolint:gosec // a document for the user
}
