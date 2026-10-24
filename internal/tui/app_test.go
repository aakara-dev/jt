package tui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jtConfig "github.com/aakara-dev/jt/internal/config"
	"github.com/aakara-dev/jt/pkg/jira"
)

type jtCtx = jtConfig.SavedProject

// fakeBackend serves canned issues and records the JQL it was asked for
// and every write it received.
type fakeBackend struct {
	mu       sync.Mutex
	issues   []*jira.Issue
	queries  []string
	calls    []string
	err      error
	writeErr error
	board    *jira.BoardConfig
	sprints  []*jira.Sprint
	totals   map[string]int // JQL → total, for count-only queries
}

func (f *fakeBackend) record(format string, a ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, a...))
	return f.writeErr
}

func (f *fakeBackend) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// statusIDs maps the sample statuses onto board column ids.
var statusIDs = map[string]string{"To Do": "1", "In Progress": "3", "In Review": "4", "Done": "10001"}

func (f *fakeBackend) Transitions(key string) ([]*jira.Transition, error) {
	var out []*jira.Transition
	for i, name := range []string{"To Do", "In Progress", "Done"} {
		t := &jira.Transition{ID: json.Number(fmt.Sprint(11 * (i + 1))), Name: "Move to " + name}
		t.To.ID, t.To.Name = statusIDs[name], name
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeBackend) Transition(key string, t *jira.Transition) error {
	return f.record("transition %s %s", key, t.Name)
}

func (f *fakeBackend) AssignableUsers(project, query string) ([]*jira.User, error) {
	return []*jira.User{{Name: "alan", DisplayName: "Alan Turing"}}, nil
}
func (f *fakeBackend) Assign(key, user string) error { return f.record("assign %s %s", key, user) }
func (f *fakeBackend) Comment(key, body string) error {
	return f.record("comment %s %s", key, body)
}

func (f *fakeBackend) Worklog(key, spent, comment string) error {
	return f.record("worklog %s %s|%s", key, spent, comment)
}

func (f *fakeBackend) Edit(key string, req *jira.EditRequest) error {
	return f.record("edit %s summary=%q priority=%q labels=%v", key, req.Summary, req.Priority, req.Labels)
}

func (f *fakeBackend) Create(req *jira.CreateRequest) (*jira.CreateResponse, error) {
	err := f.record("create %s %s %q prio=%s assignee=%s epic=%s", req.Project, req.IssueType, req.Summary, req.Priority, req.Assignee, req.ParentIssueKey)
	return &jira.CreateResponse{Key: "DATA-99"}, err
}

func (f *fakeBackend) Watch(key, user string, on bool) error {
	return f.record("watch %s %s %v", key, user, on)
}

func (f *fakeBackend) LinkTypes() ([]*jira.IssueLinkType, error) {
	return []*jira.IssueLinkType{{Name: "Blocks", Inward: "is blocked by", Outward: "blocks"}}, nil
}

func (f *fakeBackend) Link(in, out, typ string) error {
	return f.record("link in=%s out=%s %s", in, out, typ)
}
func (f *fakeBackend) Delete(key string) error { return f.record("delete %s", key) }

func (f *fakeBackend) Board(id int) (*jira.BoardConfig, error) {
	if f.board == nil {
		return nil, fmt.Errorf("no board %d", id)
	}
	return f.board, nil
}

func (f *fakeBackend) Sprints(boardID int, states string) ([]*jira.Sprint, error) {
	return f.sprints, nil
}

func (f *fakeBackend) SprintIssues(id int, jql string) (*jira.SearchResult, error) {
	return &jira.SearchResult{Issues: f.issues, Total: len(f.issues)}, nil
}

func (f *fakeBackend) BoardIssues(id int, jql string) (*jira.SearchResult, error) {
	return &jira.SearchResult{Issues: f.issues, Total: len(f.issues)}, nil
}

func (f *fakeBackend) MoveToSprint(id int, keys []string) error {
	return f.record("sprint %d %v", id, keys)
}
func (f *fakeBackend) MoveToBacklog(keys []string) error { return f.record("backlog %v", keys) }
func (f *fakeBackend) Rank(keys []string, before, after string) error {
	return f.record("rank %v before=%s after=%s", keys, before, after)
}

func (f *fakeBackend) Search(jql string, from, limit uint) (*jira.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, jql)
	if f.err != nil {
		return nil, f.err
	}
	if t, ok := f.totals[jql]; ok {
		return &jira.SearchResult{Total: t}, nil
	}
	end := min(int(from+limit), len(f.issues))
	return &jira.SearchResult{StartAt: int(from), Total: len(f.issues), Issues: f.issues[from:end]}, nil
}

func (f *fakeBackend) Issue(key string) (*jira.Issue, error) {
	for _, iss := range f.issues {
		if iss.Key == key {
			return iss, nil
		}
	}
	return nil, fmt.Errorf("issue %s not found", key)
}

func (f *fakeBackend) FavouriteFilters() ([]*jira.SavedFilter, error) {
	return []*jira.SavedFilter{{ID: "10", Name: "Team board", JQL: "project = DATA AND labels = team"}}, nil
}

func (f *fakeBackend) lastQuery() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queries) == 0 {
		return ""
	}
	return f.queries[len(f.queries)-1]
}

func issue(key, summary, status, category string) *jira.Issue {
	iss := &jira.Issue{Key: key}
	iss.Fields.Summary = summary
	iss.Fields.Status.Name = status
	iss.Fields.Status.Category.Key = category
	iss.Fields.Status.ID = statusIDs[status]
	iss.Fields.IssueType.Name = "Story"
	iss.Fields.Priority.Name = "High"
	iss.Fields.Assignee.Name = "Ada Lovelace"
	iss.Fields.Updated = time.Now().Add(-2 * time.Hour).Format(jira.RFC3339)
	iss.Fields.Description = "Load *orders* into the lake."
	return iss
}

func sampleIssues(n int) []*jira.Issue {
	out := []*jira.Issue{
		issue("DATA-1", "Add retry to the loader", "In Progress", "indeterminate"),
		issue("DATA-2", "Fix nightly orders job", "To Do", "new"),
		issue("DATA-3", "Document lineage", "Done", "done"),
	}
	for i := 4; i <= n; i++ {
		out = append(out, issue(fmt.Sprintf("DATA-%d", i), fmt.Sprintf("Backfill partition %d", i), "To Do", "new"))
	}
	return out
}

func testSession() Session {
	return Session{
		Server:  "https://jira.example.com",
		Login:   "aakara",
		Name:    "data",
		Project: "DATA",
		Queries: map[string]string{"blocked": "project = {project} AND status = Blocked"},
		Projects: []jtCtx{
			{Name: "data", Key: "DATA", Current: true},
			{Name: "ops", Key: "OPS"},
		},
	}
}

// harness drives an App synchronously.
type harness struct {
	t   *testing.T
	app *App
}

func newHarness(t *testing.T, b Backend, opts Options) *harness {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if opts.Start == "" {
		opts.Start = "issues"
	}
	h := &harness{t: t, app: NewApp(testSession(), b, opts)}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	h.run(h.app.Init(), 0)
	return h
}

// run executes cmd and feeds resulting messages back into the app. Commands
// that do not finish within wait (ticks, debounces) are dropped.
func (h *harness) run(cmd tea.Cmd, wait time.Duration) {
	if cmd == nil {
		return
	}
	if wait == 0 {
		wait = 100 * time.Millisecond
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(wait):
		return
	}
	if msg == nil {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			h.run(c, wait)
		}
		return
	}
	// tea.Sequence produces an unexported slice of commands.
	if rv := reflect.ValueOf(msg); rv.Kind() == reflect.Slice && rv.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
		for i := 0; i < rv.Len(); i++ {
			h.run(rv.Index(i).Interface().(tea.Cmd), wait)
		}
		return
	}
	if _, ok := msg.(tea.QuitMsg); ok {
		return
	}
	h.send(msg)
}

func (h *harness) send(msg tea.Msg) {
	_, cmd := h.app.Update(msg)
	h.run(cmd, 0)
}

func (h *harness) sendWait(msg tea.Msg, wait time.Duration) {
	_, cmd := h.app.Update(msg)
	h.run(cmd, wait)
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+k":
		return tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}
	case "ctrl+p":
		return tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func (h *harness) screen() string { return ansiStrip(h.app.render()) }

func TestStartsOnProjectList(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})

	assert.Equal(t, `project = "DATA" ORDER BY updated DESC`, b.lastQuery())
	s := h.screen()
	assert.Contains(t, s, "Issues · DATA (3)")
	assert.Contains(t, s, "DATA-1")
	assert.Contains(t, s, "Add retry to the loader")
	assert.Contains(t, s, "In Progress")
	assert.Contains(t, s, "jt")
	assert.Contains(t, s, "data", "status bar shows the saved project")
}

func TestNavigateToDetailAndBack(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})

	h.send(key("j"))
	assert.Equal(t, "DATA-2", h.app.top().Selected().Key)

	h.send(key("enter"))
	require.IsType(t, &detailView{}, h.app.top())
	s := h.screen()
	assert.Contains(t, s, "Fix nightly orders job")
	assert.Contains(t, s, "Ada Lovelace")
	assert.Contains(t, s, "Issues · DATA › DATA-2", "breadcrumbs")
	assert.Equal(t, "DATA-2", h.app.recent.issues[0].Key, "viewed issue is remembered")

	h.send(key("esc"))
	require.IsType(t, &listView{}, h.app.top())
}

func TestGGAndG(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(30)}, Options{})
	h.send(key("G"))
	assert.Equal(t, "DATA-30", h.app.top().Selected().Key)
	h.send(key("g"))
	h.send(key("g"))
	assert.Equal(t, "DATA-1", h.app.top().Selected().Key)
}

func TestInlineFilter(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(10)}, Options{})
	h.send(key("/"))
	assert.True(t, h.app.top().Capturing())
	h.typeText("lineage")
	assert.Equal(t, "DATA-3", h.app.top().Selected().Key)
	assert.Contains(t, h.screen(), "(1/10)")
	h.send(key("esc"))
	assert.False(t, h.app.top().Capturing())
	assert.Contains(t, h.screen(), "(10)")
}

func TestCommandLineSwitchesList(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})

	h.send(key(":"))
	require.IsType(t, &cmdline{}, h.app.popup)
	h.typeText("mine")
	h.send(key("enter"))

	assert.Nil(t, h.app.popup)
	assert.Equal(t, "assignee = currentUser() AND resolution = Unresolved ORDER BY updated DESC", b.lastQuery())
	assert.Contains(t, h.screen(), "Assigned to me")
	assert.Len(t, h.app.stack, 1, "lists replace the root like k9s")
}

func TestCommandLineSavedQueryAndIssueKey(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})

	h.send(runCommandMsg{"query blocked"})
	assert.Equal(t, "project = DATA AND status = Blocked", b.lastQuery())

	h.send(runCommandMsg{"data-3"})
	require.IsType(t, &detailView{}, h.app.top())
	assert.Contains(t, h.screen(), "Document lineage")

	h.send(runCommandMsg{"nonsense"})
	assert.Contains(t, h.screen(), "unknown command: nonsense")
}

func TestLeaderMenu(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})

	h.send(key("space"))
	require.IsType(t, whichKey{}, h.app.popup)
	s := h.screen()
	assert.Contains(t, s, "<leader>")
	assert.Contains(t, s, "active sprint")

	h.send(key("s"))
	assert.Nil(t, h.app.popup)
	assert.Equal(t, `project = "DATA" AND sprint in openSprints() ORDER BY status ASC, rank ASC`, b.lastQuery())
}

func TestSearchText(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})

	h.send(key("ctrl+k"))
	p, ok := h.app.popup.(*searchPopup)
	require.True(t, ok)
	assert.Contains(t, h.screen(), "start typing")

	for _, r := range "orders" {
		h.sendWait(tea.KeyPressMsg{Code: r, Text: string(r)}, 400*time.Millisecond)
	}
	assert.Equal(t, `project = "DATA" AND text ~ "orders*" ORDER BY updated DESC`, b.lastQuery())
	assert.Equal(t, "text", p.mode().name)
	assert.NotEmpty(t, p.results)
	assert.Contains(t, h.screen(), "search")

	h.send(key("enter"))
	assert.Nil(t, h.app.popup)
	require.IsType(t, &detailView{}, h.app.top())
}

func TestSearchModes(t *testing.T) {
	p := &searchPopup{sess: testSession()}
	cases := map[string]string{
		"#12":          "key = DATA-12",
		"#ops-7":       "key = OPS-7",
		"#nope":        "",
		"@me":          `project = "DATA" AND (assignee = currentUser() OR reporter = currentUser()) ORDER BY updated DESC`,
		`@bob`:         `project = "DATA" AND (assignee = "bob" OR reporter = "bob") ORDER BY updated DESC`,
		"> status = 1": "status = 1",
		"!lake":        `project in ("DATA", "OPS") AND text ~ "lake*" ORDER BY updated DESC`,
		`say "hi"`:     `project = "DATA" AND text ~ "say \"hi\"" ORDER BY updated DESC`,
		"data-2":       `project = "DATA" AND (text ~ "data-2*" OR key = DATA-2) ORDER BY updated DESC`,
	}
	for q, want := range cases {
		p.input.SetValue(q)
		assert.Equal(t, want, p.buildJQL(), q)
	}
}

func TestSearchKeyJumpAndSaved(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})

	h.send(runCommandMsg{"search #3"})
	h.send(key("enter"))
	require.IsType(t, &detailView{}, h.app.top())
	assert.Equal(t, "DATA-3", h.app.top().Selected().Key)

	h.send(runCommandMsg{"search *"})
	p := h.app.popup.(*searchPopup)
	require.Len(t, p.results, 2, "config query + favourite filter")
	assert.Equal(t, "blocked", p.results[0].query.Label)
	assert.Equal(t, "Team board", p.results[1].query.Label)
	h.send(key("down"))
	h.send(key("enter"))
	assert.Equal(t, "project = DATA AND labels = team", b.lastQuery())
}

func TestSearchShowsRecentWhenEmpty(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})
	h.send(runCommandMsg{"DATA-2"})
	h.send(key("esc"))

	h.send(key("ctrl+k"))
	p := h.app.popup.(*searchPopup)
	require.Len(t, p.results, 1)
	assert.Equal(t, "DATA-2", p.results[0].issue.Key)
	assert.Contains(t, h.screen(), "recently viewed")
}

func TestProjectPickerOpens(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(1)}, Options{})
	h.send(key("ctrl+p"))
	pk, ok := h.app.popup.(*picker)
	require.True(t, ok)
	assert.Equal(t, 0, pk.cursor, "cursor starts on the current project")
	s := h.screen()
	assert.Contains(t, s, "projects")
	assert.Contains(t, s, "ops")
}

func TestSessionChangeResetsStack(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2)}
	h := newHarness(t, b, Options{})
	h.send(runCommandMsg{"DATA-1"})
	require.Len(t, h.app.stack, 2)

	s := testSession()
	s.Name, s.Project = "ops", "OPS"
	h.send(sessionChangedMsg{s})
	assert.Len(t, h.app.stack, 1)
	assert.Equal(t, `project = "OPS" ORDER BY updated DESC`, b.lastQuery())
	assert.Contains(t, h.screen(), "project ops · OPS")
}

func TestStartOnIssue(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(3)}, Options{Start: "data-1"})
	require.IsType(t, &detailView{}, h.app.top())
	assert.Contains(t, h.screen(), "Add retry to the loader")
}

func TestErrorsAreToasted(t *testing.T) {
	h := newHarness(t, &fakeBackend{err: fmt.Errorf("401 unauthorized")}, Options{})
	s := h.screen()
	assert.Contains(t, s, "401 unauthorized")
}

func TestHelpPopup(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(1)}, Options{})
	h.send(key("?"))
	s := h.screen()
	assert.Contains(t, s, "Global")
	assert.Contains(t, s, "This view")
	assert.Contains(t, s, "toggle preview")
	h.send(key("esc"))
	assert.Nil(t, h.app.popup)
}

func TestTabCyclesLists(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(1)}
	h := newHarness(t, b, Options{})
	h.send(key("tab")) // issues → (board skipped: no board) → sprint
	assert.Contains(t, b.lastQuery(), "openSprints()")
	h.send(key("tab"))
	assert.Contains(t, b.lastQuery(), "sprint is EMPTY")
}

func TestQuitFromRoot(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(1)}, Options{})
	_, cmd := h.app.Update(key("q"))
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestHumanAge(t *testing.T) {
	assert.Equal(t, "now", humanAge(10*time.Second))
	assert.Equal(t, "5m", humanAge(5*time.Minute))
	assert.Equal(t, "3h", humanAge(3*time.Hour))
	assert.Equal(t, "2d", humanAge(50*time.Hour))
	assert.Equal(t, "4w", humanAge(30*24*time.Hour))
	assert.True(t, strings.HasSuffix(humanAge(800*24*time.Hour), "y"))
}
