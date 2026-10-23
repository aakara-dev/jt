package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aakara-dev/jt/pkg/jira"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aakara-dev/jt/internal/theme"
)

const toastTTL = 3 * time.Second

// cycleViews are the lists <tab> rotates through.
var cycleViews = []string{"home", "issues", "board", "sprint", "backlog", "recent"}

// App is the root Bubble Tea model: a stack of views, an optional floating
// popup, and a status bar.
type App struct {
	sess    Session
	backend Backend
	st      *styles
	recent  *recentStore

	stack []View
	popup Popup

	width, height int

	toast    string
	toastErr bool
	toastID  int

	history []string
	cycle   int

	startSearch bool
	searchInput string

	pending pendingEdit // description being edited in $EDITOR

	watchSeen   map[string]string // issue key → last seen "updated"
	watchPrimed bool
}

// Options configure how the TUI starts.
type Options struct {
	// Start is a ":" command to run first (e.g. "mine" or "DATA-12").
	Start string
	// Search opens the floating search with this input on startup.
	Search     string
	openSearch bool
}

// WithSearch opens the floating search with input on startup.
func (o Options) WithSearch(input string) Options {
	o.Search, o.openSearch = input, true
	return o
}

// NewApp builds the root model.
func NewApp(sess Session, b Backend, opts Options) *App {
	st := newStyles(sess.Theme)
	activeServer = sess.Server
	a := &App{
		sess: sess, backend: b, st: &st, recent: loadRecent(),
		startSearch: opts.openSearch, searchInput: opts.Search,
		watchSeen: map[string]string{},
	}
	start := opts.Start
	if start == "" {
		start = defaultStart
	}
	for i, name := range cycleViews {
		if name == start {
			a.cycle = i
		}
	}
	if start == "home" {
		a.stack = []View{newHomeView(sess, b, a.st)}
	} else if spec, ok := BuiltinList(start, sess); ok {
		a.stack = []View{newListView(spec, b, a.st)}
	} else if issueKeyRe.MatchString(start) {
		a.stack = []View{newDetailView(strings.ToUpper(start), b, a.st)}
	} else {
		spec, _ := BuiltinList(a.defaultList(), sess)
		a.stack = []View{newListView(spec, b, a.st)}
	}
	return a
}

func (a *App) defaultList() string {
	if a.sess.Project == "" {
		return "mine"
	}
	return "issues"
}

// defaultStart is the view jt opens on: the dashboard, unless tui.start says
// otherwise.
const defaultStart = "home"

func (a *App) top() View { return a.stack[len(a.stack)-1] }

// Init starts the first view.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.top().Init()}
	if a.startSearch {
		cmds = append(cmds, a.openSearch(a.searchInput))
	}
	if a.sess.WatchInterval > 0 {
		cmds = append(cmds, a.pollWatched()) // primes the baseline
	}
	return tea.Batch(cmds...)
}

// Update routes messages to the popup, global key handlers and views.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a, nil

	case pushViewMsg:
		a.stack = append(a.stack, msg.v)
		return a, msg.v.Init()
	case popViewMsg:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
		}
		return a, nil
	case replaceViewMsg:
		a.stack = []View{msg.v}
		return a, msg.v.Init()

	case openPopupMsg:
		a.popup = msg.p
		return a, nil
	case closePopupMsg:
		a.popup = nil
		return a, nil

	case toastMsg:
		a.toastID++
		a.toast, a.toastErr = msg.text, msg.err
		id := a.toastID
		return a, tea.Tick(toastTTL, func(time.Time) tea.Msg { return clearToastMsg{id} })
	case clearToastMsg:
		if msg.id == a.toastID {
			a.toast = ""
		}
		return a, nil

	case runCommandMsg:
		return a, a.exec(msg.line)

	case sessionChangedMsg:
		a.sess = msg.s
		activeServer = a.sess.Server
		spec, _ := BuiltinList(a.defaultList(), a.sess)
		v := newListView(spec, a.backend, a.st)
		a.stack = []View{v}
		return a, tea.Batch(v.Init(), toast("project "+a.sess.Name+" · "+a.sess.Project))

	case viewedIssueMsg:
		a.recent.add(msg.iss)
		return a, nil

	case watchTickMsg:
		return a, a.pollWatched()
	case watchResultMsg:
		return a, a.onWatchResult(msg)

	case pendingEditMsg:
		a.pending = msg.p
		return a, nil
	case editorDoneMsg:
		if msg.id == a.pending.id {
			return a, a.finishEdit(msg)
		}

	case tea.KeyPressMsg:
		return a, a.handleKey(msg)
	}

	// Async results go to every view (and the popup); each one ignores
	// messages that are not addressed to it.
	var cmds []tea.Cmd
	if a.popup != nil {
		var cmd tea.Cmd
		a.popup, cmd = a.popup.Update(msg)
		cmds = append(cmds, cmd)
	}
	for i, v := range a.stack {
		var cmd tea.Cmd
		a.stack[i], cmd = v.Update(msg)
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

func (a *App) handleKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == keyQuit {
		return tea.Quit
	}
	if a.popup != nil {
		var cmd tea.Cmd
		a.popup, cmd = a.popup.Update(k)
		return cmd
	}
	top := a.top()
	if owner, ok := top.(keyOwner); top.Capturing() || (ok && owner.Owns(key)) {
		var cmd tea.Cmd
		a.stack[len(a.stack)-1], cmd = top.Update(k)
		return cmd
	}

	switch key {
	case keyCmdline:
		a.popup = newCmdline(a.completions, &a.history)
		return nil
	case keyLeader:
		a.popup = whichKey{}
		return nil
	case keySearch:
		return a.openSearch("")
	case keyHelp:
		a.popup = helpPopup{view: append(top.Hints(), actionHints...)}
		return nil
	case "n":
		return a.createAction()
	case keyProjectSw:
		return a.exec("project")
	case keyNextView:
		a.cycle = (a.cycle + 1) % len(cycleViews)
		if cycleViews[a.cycle] == "board" && a.sess.BoardID == 0 {
			a.cycle = (a.cycle + 1) % len(cycleViews)
		}
		return a.exec(cycleViews[a.cycle])
	case keyBack, keyBackAlt:
		if key == keyBack && len(top.Marked()) > 0 {
			var cmd tea.Cmd
			a.stack[len(a.stack)-1], cmd = top.Update(k) // clears the visual selection
			return cmd
		}
		if len(a.stack) > 1 {
			return pop
		}
		if key == keyBackAlt {
			return tea.Quit
		}
		return nil
	}

	if actionKeys[key] {
		targets := top.Marked()
		if len(targets) == 0 {
			if iss := top.Selected(); iss != nil {
				targets = []*jira.Issue{iss}
			}
		}
		if len(targets) == 0 {
			return toast("no issue selected")
		}
		return a.issueAction(key, targets)
	}

	if iss := top.Selected(); iss != nil {
		switch key {
		case keyOpen:
			return openInBrowser(a.sess, iss.Key)
		case keyCopyKey:
			return copyText(iss.Key, "copied "+iss.Key)
		case keyCopyURL:
			return copyText(a.sess.BrowseURL(iss.Key), "copied URL")
		case keyCopyBr:
			return copyBranch(iss)
		}
	}

	var cmd tea.Cmd
	a.stack[len(a.stack)-1], cmd = top.Update(k)
	return cmd
}

func (a *App) openSearch(initial string) tea.Cmd {
	p, cmd := newSearchPopup(a.sess, a.backend, a.st, a.recent, initial)
	a.popup = p
	return cmd
}

// exec runs a ":" command line.
func (a *App) exec(line string) tea.Cmd {
	line = strings.TrimSpace(strings.TrimPrefix(line, ":"))
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)

	switch name {
	case "":
		return nil
	case "q", "quit", "q!", "qa":
		return tea.Quit
	case "help", "h":
		a.popup = helpPopup{view: append(a.top().Hints(), actionHints...)}
		return nil
	case "search", "s", "find":
		return a.openSearch(arg)
	case "project", "p", "proj", "ctx":
		if arg == "" {
			return a.projectPicker()
		}
		return switchProject(arg)
	case "theme":
		if arg == "" {
			return a.themePicker()
		}
		return a.setTheme(arg)
	case "export", "x":
		return a.exportView(arg)
	case "home", "dash", "dashboard", "work":
		v := newHomeView(a.sess, a.backend, a.st)
		return func() tea.Msg { return replaceViewMsg{v} }
	case "board", "kanban", "b":
		if a.sess.BoardID == 0 {
			return toast("no board for " + a.sess.Project + ": save one with `jt project add NAME KEY --board ...`")
		}
		v := newBoardView(a.sess.BoardID, a.backend, a.st)
		return func() tea.Msg { return replaceViewMsg{v} }
	case "epic":
		if arg == "" {
			return a.replace(BuiltinList("epics", a.sess))
		}
		return push(newListView(EpicChildren(strings.ToUpper(arg)), a.backend, a.st))
	case "query", "jql":
		if arg == "" {
			return a.openSearch("*")
		}
		if jql, ok := a.sess.Queries[arg]; ok {
			return push(newListView(QueryList(arg, a.sess.ExpandJQL(jql)), a.backend, a.st))
		}
		return push(newListView(QueryList("JQL", arg), a.backend, a.st))
	case "save":
		lv, ok := a.top().(*listView)
		if !ok {
			return toast("open a list to save its query")
		}
		if arg == "" {
			return toastErr(fmt.Errorf("usage: :save NAME"))
		}
		if err := SaveQuery(arg, lv.spec.JQL); err != nil {
			return toastErr(err)
		}
		a.sess.Queries[arg] = lv.spec.JQL
		return toast("saved query " + arg)
	case "open":
		if issueKeyRe.MatchString(arg) {
			return openInBrowser(a.sess, strings.ToUpper(arg))
		}
	}

	if spec, ok := BuiltinList(name, a.sess); ok {
		return a.replace(spec, true)
	}
	if issueKeyRe.MatchString(name) {
		return push(newDetailView(strings.ToUpper(name), a.backend, a.st))
	}
	return toastErr(fmt.Errorf("unknown command: %s", name))
}

// replace swaps the root list for spec, keeping the stack shallow like k9s.
func (a *App) replace(spec ListSpec, ok bool) tea.Cmd {
	if !ok {
		return nil
	}
	v := newListView(spec, a.backend, a.st)
	return func() tea.Msg { return replaceViewMsg{v} }
}

func switchProject(name string) tea.Cmd {
	return func() tea.Msg {
		s, err := SwitchProject(name)
		if err != nil {
			return toastMsg{text: err.Error(), err: true}
		}
		return sessionChangedMsg{s}
	}
}

func (a *App) projectPicker() tea.Cmd {
	if len(a.sess.Projects) == 0 {
		return toast("no saved projects: add one with `jt project add NAME KEY`")
	}
	items := make([]pickItem, 0, len(a.sess.Projects))
	for _, c := range a.sess.Projects {
		detail := c.Key
		if c.Board != "" {
			detail += " · " + c.Board
		}
		if c.Current {
			detail += "  (current)"
		}
		items = append(items, pickItem{Label: c.Name, Detail: detail, Value: c.Name})
	}
	a.popup = newPicker("projects", items, func(it pickItem) tea.Cmd {
		return switchProject(it.Value)
	}).cursorTo(a.sess.Name)
	return nil
}

func (a *App) themePicker() tea.Cmd {
	var items []pickItem
	for _, n := range theme.Names() {
		items = append(items, pickItem{Label: n, Value: n})
	}
	a.popup = newPicker("theme", items, func(it pickItem) tea.Cmd { return a.setTheme(it.Value) }).cursorTo(theme.Get(a.sess.Theme).Name)
	return nil
}

func (a *App) setTheme(name string) tea.Cmd {
	if theme.Get(name).Name != strings.ToLower(name) {
		return toastErr(fmt.Errorf("unknown theme %q (have: %s)", name, strings.Join(theme.Names(), ", ")))
	}
	*a.st = newStyles(name)
	a.sess.Theme = name
	if err := SetTheme(name); err != nil {
		return toastErr(err)
	}
	return toast("theme " + name)
}

// completions feeds the ":" prompt.
func (a *App) completions(input string) []string {
	cands := []string{"home", "export csv", "export md", "project", "board", "theme", "search", "query", "save", "epic", "open", "help", "quit", "project -"}
	cands = append(cands, BuiltinListNames...)
	for _, c := range a.sess.Projects {
		cands = append(cands, "project "+c.Name)
	}
	for _, n := range theme.Names() {
		cands = append(cands, "theme "+n)
	}
	for n := range a.sess.Queries {
		cands = append(cands, "query "+n)
	}
	return fuzzyFilter(strings.TrimPrefix(input, ":"), cands)
}

// View renders the stack's top view, the status bar and any popup.
func (a *App) View() tea.View {
	v := tea.NewView(a.render())
	v.AltScreen = true
	v.WindowTitle = "jt · " + a.sess.Name
	return v
}

func (a *App) render() string {
	if a.width == 0 {
		return ""
	}
	st := *a.st
	bar := a.statusBar()
	bodyH := a.height - lipgloss.Height(bar)
	body := lipgloss.NewStyle().Width(a.width).Height(bodyH).MaxHeight(bodyH).Render(a.top().Render(a.width, bodyH))
	screen := body + "\n" + bar
	if a.popup != nil {
		screen = placeCenter(screen, a.popup.Render(st, a.width, a.height), a.width, a.height)
	}
	return screen
}

func (a *App) statusBar() string {
	st := *a.st
	name := a.sess.Name
	if name == "" {
		name = "no project"
	}
	left := st.StatusKey.Render("jt") + st.StatusCtx.Render(name)
	if a.sess.Project != "" {
		left += st.StatusDim.Render(a.sess.Project)
	}
	var crumbs []string
	for _, v := range a.stack {
		crumbs = append(crumbs, v.Title())
	}
	left += st.StatusDim.Render(strings.Join(crumbs, " › "))

	right := st.StatusDim.Render("space menu · : cmd · ctrl+k search · ? help")
	if a.toast != "" {
		if a.toastErr {
			right = st.ToastErr.Render(a.toast)
		} else {
			right = st.ToastInfo.Render(a.toast)
		}
	}
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		left = fit(left, max(a.width-lipgloss.Width(right)-1, 0))
		gap = max(a.width-lipgloss.Width(left)-lipgloss.Width(right), 0)
	}
	return left + st.StatusBar.Render(strings.Repeat(" ", gap)) + right
}
