package tui

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/aakara-dev/jt/internal/theme"
	"github.com/aakara-dev/jt/pkg/jira"
)

// TestScreens renders representative screens with a fake backend for
// eyeballing the look and feel. It only runs when JT_SCREENS names an
// output directory:
//
//	JT_SCREENS=docs/screens go test ./internal/tui -run TestScreens
//
// Each screen is written as .ans (raw ANSI, view with `cat`), .txt, and all
// of them together as index.html.
func TestScreens(t *testing.T) {
	dir := os.Getenv("JT_SCREENS")
	if dir == "" {
		t.Skip("set JT_SCREENS=<dir> to render screens")
	}
	if !filepath.IsAbs(dir) {
		wd, _ := os.Getwd()
		dir = filepath.Join(wd, "..", "..", dir)
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))

	type shot struct{ name, title, ansi string }
	var shots []shot
	snap := func(h *harness, name, title string) {
		shots = append(shots, shot{name, title, h.app.render()})
	}
	typeSlow := func(h *harness, s string) {
		for _, r := range s {
			h.sendWait(tea.KeyPressMsg{Code: r, Text: string(r)}, 400*time.Millisecond)
		}
	}

	themeName := os.Getenv("JT_SCREENS_THEME")
	newH := func(opts Options) *harness {
		end := time.Now().Add(4 * 24 * time.Hour).Format(time.RFC3339)
		fb := &fakeBackend{issues: demoIssues(), board: demoBoard(), sprints: []*jira.Sprint{{ID: 7, Name: "DATA Sprint 42", Status: "active", EndDate: end}}}
		h := newHarness(t, fb, opts)
		h.app.sess.BoardID = 5
		if themeName != "" {
			*h.app.st = newStyles(themeName)
		}
		h.send(tea.WindowSizeMsg{Width: 140, Height: 34})
		return h
	}

	h := newH(Options{})
	snap(h, "01-issues", "Issue list with preview (start screen)")

	h.send(key("space"))
	snap(h, "02-leader", "Leader menu (space)")
	h.send(key("esc"))

	h.send(key("ctrl+k"))
	typeSlow(h, "orders")
	snap(h, "03-search-text", "Floating search: text mode (ctrl+k)")
	h.send(key("esc"))

	h.send(runCommandMsg{"search #"})
	typeSlow(h, "7")
	snap(h, "04-search-key", "Floating search: key jump (#7)")
	h.send(key("esc"))

	h.send(runCommandMsg{"search *"})
	snap(h, "05-search-saved", "Floating search: saved queries & favourite filters (*)")
	h.send(key("esc"))

	h.send(key(":"))
	h.typeText("pro")
	snap(h, "06-cmdline", "Command line with completion (:)")
	h.send(key("esc"))

	h.send(key("ctrl+p"))
	snap(h, "07-projects", "Project switcher (ctrl+p / :project)")
	h.send(key("esc"))

	h.send(key("/"))
	h.typeText("orders")
	snap(h, "08-filter", "Inline fuzzy filter (/)")
	h.send(key("esc"))

	h.send(key("enter"))
	snap(h, "09-detail", "Issue detail")
	h.send(key("esc"))

	h.send(key("?"))
	snap(h, "10-help", "Help (?)")
	h.send(key("esc"))

	h.send(runCommandMsg{"theme"})
	snap(h, "11-theme", "Theme picker (:theme)")
	h.send(key("esc"))

	h.send(key("t"))
	snap(h, "12-transition", "Transition picker (t)")
	h.send(key("esc"))

	h.send(key("v"))
	h.send(key("v"))
	h.send(key("j"))
	h.send(key("v"))
	snap(h, "13-marked", "Visual marks for bulk actions (v)")
	h.send(key("esc"))

	h.send(key("n"))
	h.typeText("Add data contract tests for orders")
	snap(h, "14-create", "New issue form (n)")
	h.send(key("esc"))

	h.send(key("c"))
	h.typeText("Retry policy looks good, ship it.")
	snap(h, "15-comment", "Comment (c)")
	h.send(key("esc"))

	h.send(runCommandMsg{"board"})
	snap(h, "16-board", "Sprint board (:board / space b)")
	h.send(key("l"))
	h.send(key("j"))
	snap(h, "17-board-nav", "Board navigation (h/l columns, j/k cards, H/L move)")

	var page strings.Builder
	page.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>jt screens</title><style>
body{background:#111218;color:#c0caf5;font-family:system-ui,sans-serif;margin:24px}
h2{font-weight:600;font-size:15px;margin:28px 0 8px;color:#a9b1d6}
pre{background:#1a1b26;padding:12px;border-radius:8px;display:inline-block;margin:0;
font:13px/1.25 "JetBrains Mono","SF Mono",Menlo,monospace;box-shadow:0 4px 24px #0008}
</style></head><body><h1>jt TUI screens</h1>`)
	for _, s := range shots {
		require.NoError(t, os.WriteFile(filepath.Join(dir, s.name+".ans"), []byte(s.ansi+"\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, s.name+".txt"), []byte(ansiStrip(s.ansi)+"\n"), 0o644))
		fmt.Fprintf(&page, "<h2>%s</h2><pre>%s</pre>\n", html.EscapeString(s.title), ansiToHTML(s.ansi))
	}
	page.WriteString("</body></html>\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte(page.String()), 0o644))
	t.Logf("wrote %d screens to %s (theme %s)", len(shots), dir, theme.Get(themeName).Name)
}

func demoIssues() []*jira.Issue {
	mk := func(key, typ, prio, summary, status, cat, who, desc string, age time.Duration, labels ...string) *jira.Issue {
		iss := issue(key, summary, status, cat)
		iss.Fields.IssueType.Name = typ
		iss.Fields.Priority.Name = prio
		iss.Fields.Assignee.Name = who
		iss.Fields.Reporter.Name = "Grace Hopper"
		iss.Fields.Labels = labels
		iss.Fields.Description = desc
		iss.Fields.Updated = time.Now().Add(-age).Format(jira.RFC3339)
		iss.Fields.Created = time.Now().Add(-age * 6).Format(jira.RFC3339)
		return iss
	}
	return []*jira.Issue{
		mk("DATA-1", "Story", "High", "Add retry with backoff to the orders loader", "In Progress", "indeterminate", "Ada Lovelace",
			"h3. Context\nThe nightly *orders* load fails on transient ADLS errors.\n\nh3. Acceptance criteria\n* retries 3x with exponential backoff\n* logs row counts per partition\n* idempotent on rerun", 2*time.Hour, "pipeline", "reliability"),
		mk("DATA-2", "Bug", "Highest", "Nightly orders job fails on NULL customer ids", "To Do", "new", "Alan Turing",
			"Spark job aborts when {{customer_id}} is NULL in the source extract.", 40*time.Minute, "bug", "orders"),
		mk("DATA-3", "Task", "Medium", "Document lineage for the finance mart", "Done", "done", "Grace Hopper", "", 3*24*time.Hour, "docs"),
		mk("DATA-4", "Epic", "Medium", "Migrate batch jobs from Oracle to Databricks", "In Progress", "indeterminate", "Ada Lovelace", "", 26*time.Hour),
		mk("DATA-5", "Story", "Low", "Expose freshness metrics in Grafana", "In Review", "indeterminate", "Linus Torvalds", "", 5*time.Hour, "observability"),
		mk("DATA-6", "Sub-task", "Medium", "Add Delta OPTIMIZE to weekly maintenance", "To Do", "new", "", "", 9*24*time.Hour),
		mk("DATA-7", "Bug", "High", "dbt incremental model duplicates late-arriving rows", "In Progress", "indeterminate", "Alan Turing", "", 30*time.Minute, "dbt"),
		mk("DATA-8", "Story", "Medium", "Salt skewed join keys in sessionisation job", "To Do", "new", "Margaret Hamilton", "", 2*24*time.Hour, "spark", "performance"),
		mk("DATA-9", "Task", "Lowest", "Clean up stale feature branches", "Done", "done", "Linus Torvalds", "", 20*24*time.Hour),
		mk("DATA-10", "Story", "High", "Partition events table by event_date", "To Do", "new", "Ada Lovelace", "", 7*time.Hour, "spark"),
		mk("DATA-11", "Task", "Medium", "Rotate service principal secret in Key Vault", "In Progress", "indeterminate", "Margaret Hamilton", "", 15*time.Minute, "security"),
		mk("DATA-12", "Story", "Medium", "Backfill 2024 partitions after schema change", "To Do", "new", "", "", 4*24*time.Hour),
	}
}

var sgrRe = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// ansiToHTML converts SGR-styled text (as produced by lipgloss) to HTML spans.
func ansiToHTML(s string) string {
	type state struct {
		fg, bg                          string
		bold, faint, italic, under, rev bool
	}
	var cur state
	var b strings.Builder
	open := false
	flush := func() {
		if open {
			b.WriteString("</span>")
			open = false
		}
		var css []string
		fg, bg := cur.fg, cur.bg
		if cur.rev {
			fg, bg = bg, fg
			if fg == "" {
				fg = "#1a1b26"
			}
			if bg == "" {
				bg = "#c0caf5"
			}
		}
		if fg != "" {
			css = append(css, "color:"+fg)
		}
		if bg != "" {
			css = append(css, "background:"+bg)
		}
		if cur.bold {
			css = append(css, "font-weight:bold")
		}
		if cur.faint {
			css = append(css, "opacity:.6")
		}
		if cur.italic {
			css = append(css, "font-style:italic")
		}
		if cur.under {
			css = append(css, "text-decoration:underline")
		}
		if len(css) > 0 {
			b.WriteString(`<span style="` + strings.Join(css, ";") + `">`)
			open = true
		}
	}
	basic := []string{"#15161e", "#f7768e", "#9ece6a", "#e0af68", "#7aa2f7", "#bb9af7", "#7dcfff", "#a9b1d6",
		"#414868", "#f7768e", "#9ece6a", "#e0af68", "#7aa2f7", "#bb9af7", "#7dcfff", "#c0caf5"}
	color256 := func(n int) string {
		switch {
		case n < 16:
			return basic[n]
		case n < 232:
			n -= 16
			lv := []int{0, 95, 135, 175, 215, 255}
			return fmt.Sprintf("#%02x%02x%02x", lv[n/36], lv[(n/6)%6], lv[n%6])
		default:
			v := 8 + (n-232)*10
			return fmt.Sprintf("#%02x%02x%02x", v, v, v)
		}
	}

	last := 0
	for _, m := range sgrRe.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(html.EscapeString(stripOtherEscapes(s[last:m[0]])))
		last = m[1]
		params := strings.FieldsFunc(s[m[2]:m[3]], func(r rune) bool { return r == ';' || r == ':' })
		if len(params) == 0 {
			params = []string{"0"}
		}
		for i := 0; i < len(params); i++ {
			n, _ := strconv.Atoi(params[i])
			switch {
			case n == 0:
				cur = state{}
			case n == 1:
				cur.bold = true
			case n == 2:
				cur.faint = true
			case n == 3:
				cur.italic = true
			case n == 4:
				cur.under = true
			case n == 7:
				cur.rev = true
			case n == 22:
				cur.bold, cur.faint = false, false
			case n == 23:
				cur.italic = false
			case n == 24:
				cur.under = false
			case n == 27:
				cur.rev = false
			case n >= 30 && n <= 37:
				cur.fg = basic[n-30]
			case n >= 90 && n <= 97:
				cur.fg = basic[n-90+8]
			case n >= 40 && n <= 47:
				cur.bg = basic[n-40]
			case n >= 100 && n <= 107:
				cur.bg = basic[n-100+8]
			case n == 39:
				cur.fg = ""
			case n == 49:
				cur.bg = ""
			case (n == 38 || n == 48) && i+1 < len(params):
				var c string
				if params[i+1] == "2" && i+4 < len(params) {
					r, _ := strconv.Atoi(params[i+2])
					g, _ := strconv.Atoi(params[i+3])
					bl, _ := strconv.Atoi(params[i+4])
					c = fmt.Sprintf("#%02x%02x%02x", r, g, bl)
					i += 4
				} else if params[i+1] == "5" && i+2 < len(params) {
					v, _ := strconv.Atoi(params[i+2])
					c = color256(v)
					i += 2
				}
				if n == 38 {
					cur.fg = c
				} else {
					cur.bg = c
				}
			}
		}
		flush()
	}
	b.WriteString(html.EscapeString(stripOtherEscapes(s[last:])))
	if open {
		b.WriteString("</span>")
	}
	return b.String()
}

var otherEscRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b\[[0-9;?]*[A-Za-ln-z]`)

func stripOtherEscapes(s string) string { return otherEscRe.ReplaceAllString(s, "") }
