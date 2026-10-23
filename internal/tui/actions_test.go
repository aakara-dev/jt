package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aakara-dev/jt/pkg/jira"
)

func TestTransitionAction(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})

	h.send(key("t"))
	pk, ok := h.app.popup.(*picker)
	require.True(t, ok, "transition picker opens")
	assert.Contains(t, h.screen(), "Move to Done")
	assert.Contains(t, h.screen(), "→ Done")

	h.typeText("done")
	h.send(key("enter"))
	assert.Nil(t, h.app.popup)
	_ = pk
	assert.Contains(t, b.callLog(), "transition DATA-1 Move to Done")
	assert.Contains(t, h.screen(), "DATA-1 → Move to Done")
}

func TestBulkTransitionWithVisualMarks(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(4)}
	h := newHarness(t, b, Options{})

	h.send(key("v")) // marks DATA-1, moves to DATA-2
	h.send(key("j")) // skip DATA-2
	h.send(key("v")) // marks DATA-3
	assert.Contains(t, h.screen(), "[2 marked]")

	h.send(key("t"))
	h.typeText("progress")
	h.send(key("enter"))
	assert.Equal(t, []string{"transition DATA-1 Move to In Progress", "transition DATA-3 Move to In Progress"}, b.callLog())

	h.send(key("esc"))
	assert.NotContains(t, h.screen(), "marked", "esc clears the visual selection")
}

func TestAssignToMeAndPicker(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2)}
	h := newHarness(t, b, Options{})

	h.send(key("A"))
	assert.Contains(t, b.callLog(), "assign DATA-1 aakara")

	h.send(key("a"))
	require.IsType(t, &asyncPicker{}, h.app.popup)
	for _, r := range "alan" {
		h.sendWait(tea.KeyPressMsg{Code: r, Text: string(r)}, 400*time.Millisecond)
	}
	assert.Contains(t, h.screen(), "Alan Turing")
	h.send(key("enter"))
	assert.Contains(t, b.callLog(), "assign DATA-1 alan")
}

func TestCommentAction(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2)}
	h := newHarness(t, b, Options{})

	h.send(key("c"))
	require.IsType(t, &textPopup{}, h.app.popup)
	h.typeText("Looks good")
	h.send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	assert.Nil(t, h.app.popup)
	assert.Contains(t, b.callLog(), "comment DATA-1 Looks good")
}

func TestWorklogAction(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2)}
	h := newHarness(t, b, Options{})

	h.send(key("w"))
	h.typeText("1h 30m reviewed PR")
	h.send(key("enter"))
	assert.Contains(t, b.callLog(), "worklog DATA-1 1h 30m|reviewed PR")

	h.send(key("w"))
	h.typeText("soon")
	h.send(key("enter"))
	assert.Contains(t, h.screen(), "start with a duration")
}

func TestParseWorklog(t *testing.T) {
	spent, comment, err := parseWorklog("2d 4h  design review")
	require.NoError(t, err)
	assert.Equal(t, "2d 4h", spent)
	assert.Equal(t, "design review", comment)

	spent, comment, err = parseWorklog("45m")
	require.NoError(t, err)
	assert.Equal(t, "45m", spent)
	assert.Equal(t, "", comment)

	_, _, err = parseWorklog("yesterday 2h")
	assert.Error(t, err)
}

func TestEditLabels(t *testing.T) {
	issues := sampleIssues(1)
	issues[0].Fields.Labels = []string{"old", "keep"}
	b := &fakeBackend{issues: issues}
	h := newHarness(t, b, Options{})

	h.send(key("e"))
	h.typeText("labels")
	h.send(key("enter"))
	p, ok := h.app.popup.(*prompt)
	require.True(t, ok)
	p.input.SetValue("keep new")
	h.send(key("enter"))
	assert.Contains(t, b.callLog(), `edit DATA-1 summary="" priority="" labels=[keep new -old]`)
}

func TestWatchToggle(t *testing.T) {
	issues := sampleIssues(1)
	issues[0].Fields.Watches.IsWatching = true
	b := &fakeBackend{issues: issues}
	h := newHarness(t, b, Options{})
	h.send(key("W"))
	assert.Contains(t, b.callLog(), "watch DATA-1 aakara false")
}

func TestLinkAction(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2)}
	h := newHarness(t, b, Options{})

	h.send(key("L"))
	h.typeText("blocked by")
	h.send(key("enter"))
	h.typeText("data-2")
	h.send(key("enter"))
	assert.Contains(t, b.callLog(), "link in=DATA-1 out=DATA-2 Blocks", "DATA-1 is blocked by DATA-2")
}

func TestDeleteNeedsConfirmation(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2)}
	h := newHarness(t, b, Options{})

	h.send(key("D"))
	require.IsType(t, confirmPopup{}, h.app.popup)
	assert.Contains(t, h.screen(), "cannot be undone")
	h.send(key("n"))
	assert.Empty(t, b.callLog())

	h.send(key("D"))
	h.send(key("y"))
	assert.Equal(t, []string{"delete DATA-1"}, b.callLog())
}

func TestCreateIssueForm(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(1)}
	h := newHarness(t, b, Options{})

	h.send(key("n"))
	f, ok := h.app.popup.(*formPopup)
	require.True(t, ok)
	assert.Equal(t, 1, f.focus, "summary is focused first")
	assert.Contains(t, h.screen(), "new issue in DATA")

	h.send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	assert.Contains(t, h.screen(), "Summary is required")

	h.typeText("Add data contract tests")
	h.send(key("tab")) // priority
	h.send(key("right"))
	h.send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	assert.Nil(t, h.app.popup)
	assert.Contains(t, b.callLog(), `create DATA Story "Add data contract tests" prio=Low assignee=aakara epic=`)
	assert.Contains(t, h.screen(), "created DATA-99")
}

func TestMoveToSprintAndBacklog(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2), sprints: []*jira.Sprint{{ID: 7, Name: "Sprint 42", Status: "active"}}}
	h := newHarnessWithBoard(t, b)

	h.send(runCommandMsg{"backlog"})
	h.send(key("m"))
	h.send(key("enter"))
	assert.Contains(t, b.callLog(), "sprint 7 [DATA-1]")

	h.send(key("B"))
	assert.Contains(t, b.callLog(), "backlog [DATA-1]")
}

func TestRankInList(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{})
	h.send(key("j"))
	h.send(key("K")) // DATA-2 above DATA-1
	assert.Contains(t, b.callLog(), "rank [DATA-2] before=DATA-1 after=")
	h.send(key("J")) // and back down
	assert.Contains(t, b.callLog(), "rank [DATA-2] before= after=DATA-1")
}

func TestWriteErrorsAreReported(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(1), writeErr: fmt.Errorf("403 forbidden")}
	h := newHarness(t, b, Options{})
	h.send(key("A"))
	assert.Contains(t, h.screen(), "DATA-1: 403 forbidden")
}

func TestHelpListsActions(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(1)}, Options{})
	h.send(key("?"))
	assert.Contains(t, h.screen(), "transition")
	assert.Contains(t, h.screen(), "mark / mark all")
}

// --- board ---------------------------------------------------------------------

func demoBoard() *jira.BoardConfig {
	cfg := &jira.BoardConfig{ID: 5, Name: "DATA board", Type: "scrum"}
	for _, c := range []struct{ name, id string }{{"To Do", "1"}, {"In Progress", "3"}, {"In Review", "4"}, {"Done", "10001"}} {
		col := jira.BoardColumn{Name: c.name}
		col.Statuses = append(col.Statuses, struct {
			ID string `json:"id"`
		}{c.id})
		cfg.ColumnConfig.Columns = append(cfg.ColumnConfig.Columns, col)
	}
	return cfg
}

func newHarnessWithBoard(t *testing.T, b *fakeBackend) *harness {
	t.Helper()
	if b.board == nil {
		b.board = demoBoard()
	}
	if b.sprints == nil {
		end := time.Now().Add(72 * time.Hour).Format(time.RFC3339)
		b.sprints = []*jira.Sprint{{ID: 7, Name: "Sprint 42", Status: "active", EndDate: end}}
	}
	h := newHarness(t, b, Options{})
	h.app.sess.BoardID = 5
	return h
}

func TestBoardGroupsByColumn(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(5)}
	h := newHarnessWithBoard(t, b)
	h.send(runCommandMsg{"board"})

	bv, ok := h.app.top().(*boardView)
	require.True(t, ok)
	require.Len(t, bv.columns, 4)
	assert.Len(t, bv.columns[0].issues, 3, "DATA-2, 4, 5 are To Do")
	assert.Len(t, bv.columns[1].issues, 1)
	assert.Empty(t, bv.columns[2].issues)
	assert.Len(t, bv.columns[3].issues, 1)

	s := h.screen()
	assert.Contains(t, s, "Sprint 42")
	assert.Contains(t, s, "TO DO 3")
	assert.Contains(t, s, "IN PROGRESS 1")
	assert.Contains(t, s, "1/5 done")
	assert.Contains(t, s, "left")
}

func TestBoardMoveCard(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarnessWithBoard(t, b)
	h.send(runCommandMsg{"board"})
	bv := h.app.top().(*boardView)

	assert.Equal(t, "DATA-2", bv.Selected().Key)
	h.send(key("L")) // To Do → In Progress
	assert.Contains(t, b.callLog(), "transition DATA-2 Move to In Progress")

	h.send(key("l"))
	h.send(key("H")) // Done → In Progress for the selected card
	assert.NotEmpty(t, b.callLog())
}

func TestBoardMoveFailureReloads(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2), writeErr: fmt.Errorf("workflow says no")}
	h := newHarnessWithBoard(t, b)
	h.send(runCommandMsg{"board"})
	h.send(key("L"))
	bv := h.app.top().(*boardView)
	assert.Len(t, bv.columns[0].issues, 1, "card is back in its column after reload")
	assert.Contains(t, h.screen(), "workflow says no")
}

func TestBoardWithoutBoardID(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(1)}, Options{})
	h.send(runCommandMsg{"board"})
	assert.Contains(t, h.screen(), "no board for DATA")
}

func TestInitials(t *testing.T) {
	assert.Equal(t, "AL", initials("Ada Lovelace"))
	assert.Equal(t, "G", initials("grace"))
	assert.Equal(t, "--", initials(" "))
	assert.Equal(t, "MH", initials("Margaret Heafield Hamilton"))
}
