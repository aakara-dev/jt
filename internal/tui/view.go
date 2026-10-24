package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/aakara-dev/jt/pkg/jira"
)

// View is one screen on the view stack (issue list, issue detail, ...).
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	Render(width, height int) string
	Title() string
	// Hints are the view's own keys, shown in the footer and in help.
	Hints() []Hint
	// Capturing is true while the view owns the keyboard (e.g. inline
	// filter input), so global keys are not intercepted.
	Capturing() bool
	// Selected is the issue under the cursor, if any; global issue actions
	// (open, copy, branch) act on it.
	Selected() *jira.Issue
	// Marked are the issues selected in visual mode; actions apply to all
	// of them when non-empty.
	Marked() []*jira.Issue
}

// keyOwner is implemented by views that use keys which are otherwise global
// (e.g. the board uses h/l/H/L for columns).
type keyOwner interface {
	Owns(key string) bool
}

// Hint is a key and what it does.
type Hint struct {
	Key, Desc string
}

// Messages shared across views.
type (
	// pushViewMsg pushes a view on the stack.
	pushViewMsg struct{ v View }
	// popViewMsg pops the top view.
	popViewMsg struct{}
	// replaceViewMsg replaces the whole stack with v.
	replaceViewMsg struct{ v View }
	// toastMsg shows a transient message in the footer.
	toastMsg struct {
		text string
		err  bool
	}
	// clearToastMsg hides a toast if it is still the one with this id.
	clearToastMsg struct{ id int }
	// openPopupMsg opens a floating window.
	openPopupMsg struct{ p Popup }
	// closePopupMsg closes the floating window.
	closePopupMsg struct{}
	// runCommandMsg executes a ":" command line.
	runCommandMsg struct{ line string }
	// sessionChangedMsg is sent after a project switch.
	sessionChangedMsg struct{ s Session }
	// viewedIssueMsg records an issue in the recent list.
	viewedIssueMsg struct{ iss *jira.Issue }
)

func push(v View) tea.Cmd       { return func() tea.Msg { return pushViewMsg{v} } }
func pop() tea.Msg              { return popViewMsg{} }
func openPopup(p Popup) tea.Cmd { return func() tea.Msg { return openPopupMsg{p} } }
func closePopup() tea.Msg       { return closePopupMsg{} }

func toast(format string) tea.Cmd {
	return func() tea.Msg { return toastMsg{text: format} }
}

func toastErr(err error) tea.Cmd {
	return func() tea.Msg { return toastMsg{text: err.Error(), err: true} }
}

func runCommand(line string) tea.Cmd {
	return func() tea.Msg { return runCommandMsg{line} }
}
