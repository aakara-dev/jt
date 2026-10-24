package tui

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aakara-dev/jt/pkg/jira"
)

func TestHomeDashboard(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(3)}
	h := newHarness(t, b, Options{Start: "home"})

	hv, ok := h.app.top().(*homeView)
	require.True(t, ok)
	s := h.screen()
	assert.Contains(t, s, "In progress")
	assert.Contains(t, s, "Up next")
	assert.Contains(t, s, "Watching")
	assert.Contains(t, h.screen(), "My work")

	// The fake returns the same issues for every section; j walks across them.
	assert.Equal(t, "DATA-1", hv.Selected().Key)
	h.send(key("j"))
	assert.Equal(t, "DATA-2", hv.Selected().Key)
	h.send(key("]"))
	assert.Equal(t, 1, hv.currentSection())
	h.send(key("["))
	assert.Equal(t, 0, hv.currentSection())

	h.send(key("O"))
	lv, ok := h.app.top().(*listView)
	require.True(t, ok)
	assert.Equal(t, "In progress", lv.spec.Title)

	h.send(key("esc"))
	h.send(key("enter"))
	require.IsType(t, &detailView{}, h.app.top())
}

func TestEpicProgress(t *testing.T) {
	epic := issue("DATA-4", "Migrate to Databricks", "In Progress", "indeterminate")
	epic.Fields.IssueType.Name = "Epic"
	b := &fakeBackend{
		issues: []*jira.Issue{epic},
		totals: map[string]int{
			`"Epic Link" = DATA-4`:                           8,
			`"Epic Link" = DATA-4 AND statusCategory = Done`: 6,
		},
	}
	h := newHarness(t, b, Options{Start: "epics"})
	lv := h.app.top().(*listView)
	assert.Equal(t, [2]int{6, 8}, lv.progress["DATA-4"])
	assert.Contains(t, h.screen(), "6/8")

	h.send(key("enter"))
	child := h.app.top().(*listView)
	assert.Equal(t, `"Epic Link" = DATA-4 ORDER BY rank ASC`, child.spec.JQL)
}

func TestListCacheShowsStaleResultsFirst(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	activeServer = "https://jira.example.com"
	spec, _ := BuiltinList("issues", testSession())
	saveListCache(activeServer, spec.JQL, sampleIssues(3))

	st := newStyles("")
	v := newListView(spec, &fakeBackend{}, &st)
	cmd := v.Init()
	assert.True(t, v.cached)
	assert.Len(t, v.issues, 3)
	require.NotNil(t, cmd)

	other := loadListCache("https://other.example.com", spec.JQL)
	assert.Empty(t, other, "cache is per server")

	cached := loadListCache(activeServer, spec.JQL)
	assert.Nil(t, cached[0].Fields.Description, "descriptions are not cached")
}

func TestWatchNotifications(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(2)}
	h := newHarness(t, b, Options{})
	h.app.sess.WatchInterval = time.Minute

	h.send(watchResultMsg{issues: sampleIssues(2)})
	assert.True(t, h.app.watchPrimed)
	assert.NotContains(t, h.screen(), "updated ·", "first poll only records a baseline")

	changedIssue := sampleIssues(2)
	changedIssue[1].Fields.Updated = time.Now().Format(jira.RFC3339)
	h.send(watchResultMsg{issues: changedIssue})
	assert.Contains(t, h.screen(), "DATA-2 updated · Fix nightly orders job")

	assert.Equal(t, "(watcher = currentUser() OR assignee = currentUser()) AND updated >= -10m ORDER BY updated DESC", watchJQL(5*time.Minute))
}

func TestExportCSVAndMarkdown(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, &fakeBackend{issues: sampleIssues(3)}, Options{})

	csvPath := filepath.Join(dir, "out.csv")
	h.send(runCommandMsg{"export " + csvPath})
	assert.Contains(t, h.screen(), "exported 3 issues")
	f, err := os.Open(csvPath)
	require.NoError(t, err)
	rows, err := csv.NewReader(f).ReadAll()
	_ = f.Close()
	require.NoError(t, err)
	require.Len(t, rows, 4)
	assert.Equal(t, exportHeader, rows[0])
	assert.Equal(t, "DATA-1", rows[1][0])

	mdPath := filepath.Join(dir, "out.md")
	h.send(runCommandMsg{"export md " + mdPath})
	md, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(md), "| Key | Type | Status"))
	assert.Contains(t, string(md), "| DATA-3 | Story | Done |")

	h.send(runCommandMsg{"export xml"})
	assert.Contains(t, h.screen(), "csv or md")
}

func TestBoardStats(t *testing.T) {
	b := &fakeBackend{issues: sampleIssues(5)}
	h := newHarnessWithBoard(t, b)
	h.send(runCommandMsg{"board"})
	h.send(key("S"))
	require.IsType(t, statsPopup{}, h.app.popup)
	s := h.screen()
	assert.Contains(t, s, "20% done")
	assert.Contains(t, s, "By assignee")
	assert.Contains(t, s, "Ada Lovelace")
}

func TestHomeHighlightsOneRowWhenIssuesRepeat(t *testing.T) {
	h := newHarness(t, &fakeBackend{issues: sampleIssues(2)}, Options{Start: "home"})
	assert.Equal(t, 1, strings.Count(h.screen(), "▌"), "same issue in every section, one cursor")
}
