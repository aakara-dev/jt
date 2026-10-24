package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aakara-dev/jt/pkg/jira"
)

// kanbanJQL limits kanban boards (which have no sprints) to open work and
// anything resolved in the last two weeks.
const kanbanJQL = "resolution = Unresolved OR resolutiondate >= -14d"

type boardLoadedMsg struct {
	id      int
	cfg     *jira.BoardConfig
	sprints []*jira.Sprint
	sprint  int // index into sprints
	issues  []*jira.Issue
	err     error
}

type boardColumn struct {
	name     string
	statuses map[string]bool
	issues   []*jira.Issue
	offset   int
}

var boardSeq int

// boardView is a kanban board: the active sprint for scrum boards, open
// work for kanban boards. h/l pick a column, j/k a card, H/L move the card
// to the previous/next column by transitioning it.
type boardView struct {
	id      int
	boardID int
	backend Backend
	st      *styles

	cfg       *jira.BoardConfig
	sprints   []*jira.Sprint
	sprintIdx int
	sprint    *jira.Sprint
	columns   []*boardColumn
	unmapped  int

	col, row int
	loading  bool
	err      error
}

func newBoardView(boardID int, b Backend, st *styles) *boardView {
	boardSeq++
	return &boardView{id: boardSeq, boardID: boardID, backend: b, st: st, loading: true}
}

func (v *boardView) Init() tea.Cmd { return v.load(-1) }

// load fetches the board layout and its issues. sprintIdx -1 keeps the
// current sprint (or picks the first active one).
func (v *boardView) load(sprintIdx int) tea.Cmd {
	v.loading = true
	id, boardID, b := v.id, v.boardID, v.backend
	if sprintIdx < 0 {
		sprintIdx = v.sprintIdx
	}
	return func() tea.Msg {
		cfg, err := b.Board(boardID)
		if err != nil {
			return boardLoadedMsg{id: id, err: err}
		}
		msg := boardLoadedMsg{id: id, cfg: cfg}
		var res *jira.SearchResult
		if strings.EqualFold(cfg.Type, "kanban") {
			res, err = b.BoardIssues(boardID, kanbanJQL)
		} else {
			msg.sprints, err = b.Sprints(boardID, "active")
			if err != nil {
				return boardLoadedMsg{id: id, err: err}
			}
			if len(msg.sprints) == 0 {
				return boardLoadedMsg{id: id, cfg: cfg, err: fmt.Errorf("no active sprint on %s", cfg.Name)}
			}
			msg.sprint = min(sprintIdx, len(msg.sprints)-1)
			res, err = b.SprintIssues(msg.sprints[msg.sprint].ID, "")
		}
		if err != nil {
			msg.err = err
			return msg
		}
		msg.issues = res.Issues
		return msg
	}
}

func (v *boardView) Title() string {
	switch {
	case v.sprint != nil:
		return v.sprint.Name
	case v.cfg != nil:
		return v.cfg.Name
	}
	return "board"
}

func (v *boardView) Capturing() bool { return false }

func (v *boardView) Owns(key string) bool {
	switch key {
	case "h", "l", "H", "L", "left", "right", "[", "]":
		return true
	}
	return false
}

func (v *boardView) Marked() []*jira.Issue { return nil }

func (v *boardView) Selected() *jira.Issue {
	if v.col >= len(v.columns) {
		return nil
	}
	c := v.columns[v.col]
	if v.row >= len(c.issues) {
		return nil
	}
	return c.issues[v.row]
}

func (v *boardView) Hints() []Hint {
	return []Hint{
		{"h / l", "previous / next column"},
		{"j / k", "next / previous card"},
		{"H / L", "move card left / right (transition)"},
		{"[ / ]", "previous / next active sprint"},
		{"S", "sprint stats"},
		{"enter", "details"},
		{"ctrl+r", "reload"},
	}
}

func (v *boardView) group(issues []*jira.Issue) {
	keep := ""
	if iss := v.Selected(); iss != nil {
		keep = iss.Key
	}
	v.columns = v.columns[:0]
	for _, c := range v.cfg.ColumnConfig.Columns {
		bc := &boardColumn{name: c.Name, statuses: map[string]bool{}}
		for _, s := range c.Statuses {
			bc.statuses[s.ID] = true
		}
		v.columns = append(v.columns, bc)
	}
	v.unmapped = 0
	for _, iss := range issues {
		placed := false
		for _, c := range v.columns {
			if c.statuses[iss.Fields.Status.ID] {
				c.issues = append(c.issues, iss)
				placed = true
				break
			}
		}
		if !placed {
			v.unmapped++
		}
	}
	v.col, v.row = min(v.col, max(len(v.columns)-1, 0)), 0
	for ci, c := range v.columns {
		for ri, iss := range c.issues {
			if iss.Key == keep {
				v.col, v.row = ci, ri
			}
		}
	}
	v.clampRow()
}

func (v *boardView) clampRow() {
	if v.col >= len(v.columns) {
		return
	}
	n := len(v.columns[v.col].issues)
	v.row = max(min(v.row, n-1), 0)
}

func (v *boardView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case boardLoadedMsg:
		if msg.id != v.id {
			return v, nil
		}
		v.loading = false
		v.err = msg.err
		if msg.cfg != nil {
			v.cfg = msg.cfg
		}
		v.sprints, v.sprintIdx, v.sprint = msg.sprints, msg.sprint, nil
		if len(msg.sprints) > 0 {
			v.sprint = msg.sprints[msg.sprint]
		}
		if msg.err != nil {
			return v, toastErr(msg.err)
		}
		v.group(msg.issues)
		return v, nil

	case issuesChangedMsg:
		return v, v.load(-1)

	case boardMoveFailedMsg:
		return v, tea.Batch(toastErr(msg.err), v.load(-1))

	case tea.KeyPressMsg:
		switch msg.String() {
		case "h", "left":
			if v.col > 0 {
				v.col--
				v.clampRow()
			}
		case "l", "right":
			if v.col < len(v.columns)-1 {
				v.col++
				v.clampRow()
			}
		case "j", "down":
			v.row++
			v.clampRow()
		case "k", "up":
			if v.row > 0 {
				v.row--
			}
		case "g", "home":
			v.row = 0
		case "G", "end":
			v.row = 1 << 30
			v.clampRow()
		case "H":
			return v, v.moveCard(-1)
		case "L":
			return v, v.moveCard(1)
		case "[":
			if v.sprintIdx > 0 {
				return v, v.load(v.sprintIdx - 1)
			}
		case "]":
			if v.sprintIdx < len(v.sprints)-1 {
				return v, v.load(v.sprintIdx + 1)
			}
		case "ctrl+r":
			return v, v.load(-1)
		case "S":
			return v, openPopup(statsPopup{title: v.Title(), columns: v.columns})
		case "enter":
			if iss := v.Selected(); iss != nil {
				return v, push(newDetailView(iss.Key, v.backend, v.st))
			}
		}
	}
	return v, nil
}

type boardMoveFailedMsg struct{ err error }

// moveCard moves the selected card one column over: optimistically on
// screen, then via a transition into one of the target column's statuses.
// On failure the board reloads, which undoes the optimistic move.
func (v *boardView) moveCard(dir int) tea.Cmd {
	iss := v.Selected()
	to := v.col + dir
	if iss == nil || to < 0 || to >= len(v.columns) {
		return nil
	}
	from := v.columns[v.col]
	target := v.columns[to]
	from.issues = append(from.issues[:v.row:v.row], from.issues[v.row+1:]...)
	target.issues = append([]*jira.Issue{iss}, target.issues...)
	v.col, v.row = to, 0

	b, statuses, name := v.backend, target.statuses, target.name
	return func() tea.Msg {
		ts, err := b.Transitions(iss.Key)
		if err != nil {
			return boardMoveFailedMsg{err}
		}
		for _, t := range ts {
			if statuses[t.To.ID] {
				if err := b.Transition(iss.Key, t); err != nil {
					return boardMoveFailedMsg{err}
				}
				return tea.BatchMsg{toast(iss.Key + " → " + name), func() tea.Msg { return changed(iss.Key) }}
			}
		}
		return boardMoveFailedMsg{fmt.Errorf("%s has no transition into %q", iss.Key, name)}
	}
}

const (
	cardHeight = 5 // border + key line + two summary lines
	minColW    = 26
)

func (v *boardView) Render(width, height int) string {
	st := *v.st
	if v.cfg == nil || len(v.columns) == 0 {
		switch {
		case v.err != nil:
			return " " + st.Error.Render(v.err.Error())
		case v.loading:
			return st.Muted.Render("  loading board…")
		}
		return st.Muted.Render("  board has no columns")
	}
	header := v.renderHeader(width)
	bodyH := height - lipgloss.Height(header) - 1

	// Show as many columns as fit, scrolled so the selected one is visible.
	n := len(v.columns)
	visible := max(min(n, width/minColW), 1)
	first := max(min(v.col-visible/2, n-visible), 0)
	colW := (width - (visible - 1)) / visible

	var cols []string
	for i := first; i < first+visible && i < n; i++ {
		cols = append(cols, v.renderColumn(i, colW, bodyH))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, intersperse(cols, " ")...)
	return header + "\n\n" + body
}

func intersperse(items []string, sep string) []string {
	out := make([]string, 0, len(items)*2)
	for i, it := range items {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, it)
	}
	return out
}

func (v *boardView) renderHeader(width int) string {
	st := *v.st
	total, done := 0, 0
	for i, c := range v.columns {
		total += len(c.issues)
		for _, iss := range c.issues {
			if isDone(iss.Fields.Status) || i == len(v.columns)-1 {
				done++
			}
		}
	}
	title := st.Title.Render(v.Title())
	if v.sprint != nil {
		if end, err := time.Parse(time.RFC3339, v.sprint.EndDate); err == nil {
			left := time.Until(end)
			when := humanAge(left) + " left"
			if left < 0 {
				when = humanAge(-left) + " overdue"
			}
			title += "  " + st.Muted.Render(when)
		}
		if len(v.sprints) > 1 {
			title += "  " + st.Muted.Render(fmt.Sprintf("sprint %d/%d  [ ]", v.sprintIdx+1, len(v.sprints)))
		}
	} else if v.cfg != nil {
		title += "  " + st.Muted.Render("kanban")
	}
	bar := progressBar(st, done, total, 24)
	right := bar + " " + st.Muted.Render(fmt.Sprintf("%d/%d done", done, total))
	if v.unmapped > 0 {
		right += st.Warn.Render(fmt.Sprintf("  %d not on board", v.unmapped))
	}
	if v.loading {
		right += " " + st.Warn.Render("⟳")
	}
	gap := max(width-lipgloss.Width(title)-lipgloss.Width(right)-2, 1)
	return " " + title + strings.Repeat(" ", gap) + right
}

func progressBar(st styles, done, total, width int) string {
	if total == 0 {
		return st.Muted.Render(strings.Repeat("─", width))
	}
	filled := done * width / total
	return st.Success.Render(strings.Repeat("━", filled)) + st.Muted.Render(strings.Repeat("─", width-filled))
}

func (v *boardView) renderColumn(i, width, height int) string {
	st := *v.st
	c := v.columns[i]
	selected := i == v.col

	head := fmt.Sprintf("%s %d", strings.ToUpper(c.name), len(c.issues))
	hs := lipgloss.NewStyle().Width(width).Padding(0, 1).Bold(true)
	if selected {
		hs = hs.Background(st.Palette.Accent).Foreground(st.Palette.Bg)
	} else {
		hs = hs.Background(st.Palette.Surface).Foreground(st.Palette.Subtle)
	}
	lines := []string{hs.Render(fit(head, width-2))}

	slots := max((height-1)/cardHeight, 1)
	if selected {
		if v.row < c.offset {
			c.offset = v.row
		}
		if v.row >= c.offset+slots {
			c.offset = v.row - slots + 1
		}
	}
	for r := c.offset; r < len(c.issues) && r < c.offset+slots; r++ {
		lines = append(lines, v.renderCard(c.issues[r], width, selected && r == v.row))
	}
	if more := len(c.issues) - c.offset - slots; more > 0 {
		lines = append(lines, st.Muted.Render(fmt.Sprintf(" ↓ %d more", more)))
	}
	if len(c.issues) == 0 {
		lines = append(lines, st.Muted.Render(" empty"))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

func (v *boardView) renderCard(iss *jira.Issue, width int, selected bool) string {
	st := *v.st
	f := iss.Fields
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(width).Padding(0, 1)
	if selected {
		border = border.BorderForeground(st.Palette.Accent).Background(st.Palette.Overlay)
	} else {
		border = border.BorderForeground(st.Palette.Overlay)
	}
	inner := width - 4
	who := initials(f.Assignee.Name)
	top := st.typeIcon(f.IssueType) + " " + st.Accent.Bold(true).Render(iss.Key)
	right := st.priorityIcon(f.Priority.Name) + " " + st.Accent2.Render(who)
	top += strings.Repeat(" ", max(inner-lipgloss.Width(top)-lipgloss.Width(right), 1)) + right

	summary := lipgloss.NewStyle().Width(inner).Render(f.Summary)
	sl := strings.Split(summary, "\n")
	for len(sl) < 2 {
		sl = append(sl, "")
	}
	if len(sl) > 2 {
		sl = sl[:2]
		sl[1] = fit(strings.TrimRight(sl[1], " ")+"…", inner)
	}
	return border.Render(top + "\n" + st.Value.Render(strings.Join(sl, "\n")))
}

// initials turns "Ada Lovelace" into "AL".
func initials(name string) string {
	if strings.TrimSpace(name) == "" {
		return "--"
	}
	var out []rune
	for _, part := range strings.Fields(name) {
		out = append(out, []rune(part)[0])
		if len(out) == 2 {
			break
		}
	}
	return strings.ToUpper(string(out))
}
