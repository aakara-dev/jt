package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/aakara-dev/jt/pkg/jira"
)

const searchDebounce = 250 * time.Millisecond

// searchMode is selected by the first character of the query.
type searchMode struct {
	prefix, name, hint string
}

var searchModes = []searchMode{
	{"", "text", "summary, description & comments in this project"},
	{">", "jql", "raw JQL"},
	{"#", "key", "jump to an issue key (#123 uses the current project)"},
	{"@", "people", "assigned to or reported by a user (@me)"},
	{"*", "saved", "saved queries and favourite filters"},
	{"!", "everywhere", "text search across all saved projects"},
}

var (
	issueKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]+-\d+$`)
	digitsRe   = regexp.MustCompile(`^\d+$`)
)

type (
	searchDebounceMsg struct{ seq int }
	searchResultMsg   struct {
		seq    int
		jql    string
		issues []*jira.Issue
		total  int
		err    error
	}
	favouritesMsg struct {
		filters []*jira.SavedFilter
		err     error
	}
)

// searchResult is a row in the search window: an issue or a saved query.
type searchResult struct {
	issue *jira.Issue
	query *pickItem // saved query: Label=name, Value=JQL
}

// searchPopup is the Telescope-style floating search window.
type searchPopup struct {
	sess    Session
	backend Backend
	st      *styles
	recent  *recentStore

	input   textinput.Model
	seq     int
	loading bool
	err     error
	jql     string
	total   int
	results []searchResult
	cursor  int

	favourites []*jira.SavedFilter
	favLoaded  bool
}

func newSearchPopup(sess Session, b Backend, st *styles, recent *recentStore, initial string) (*searchPopup, tea.Cmd) {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "search issues…  (> jql  # key  @ user  * saved  ! everywhere)"
	in.SetValue(initial)
	in.CursorEnd()
	focus := in.Focus()
	p := &searchPopup{sess: sess, backend: b, st: st, recent: recent, input: in}
	return p, tea.Batch(focus, p.onInput())
}

func (p *searchPopup) mode() searchMode { return modeOf(p.input.Value()) }

func modeOf(q string) searchMode {
	for _, m := range searchModes[1:] {
		if strings.HasPrefix(q, m.prefix) {
			return m
		}
	}
	return searchModes[0]
}

func (p *searchPopup) query() string { return queryOf(p.input.Value()) }

func queryOf(input string) string {
	return strings.TrimSpace(strings.TrimPrefix(input, modeOf(input).prefix))
}

// onInput reacts to a query change: instant local results where possible,
// otherwise a debounced server search.
func (p *searchPopup) onInput() tea.Cmd {
	p.seq++
	p.cursor = 0
	p.err = nil
	m, q := p.mode(), p.query()

	switch {
	case m.name == "saved":
		p.loading = false
		p.results = p.savedResults(q)
		if !p.favLoaded {
			p.favLoaded = true
			b := p.backend
			return func() tea.Msg {
				f, err := b.FavouriteFilters()
				return favouritesMsg{f, err}
			}
		}
		return nil
	case q == "" && m.name == "text":
		p.loading = false
		p.jql = ""
		p.results = p.recentResults()
		return nil
	case q == "":
		p.loading = false
		p.results = nil
		return nil
	}
	seq := p.seq
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg { return searchDebounceMsg{seq} })
}

func (p *searchPopup) recentResults() []searchResult {
	out := make([]searchResult, 0, len(p.recent.issues))
	for _, iss := range p.recent.issues {
		out = append(out, searchResult{issue: iss})
	}
	return out
}

func (p *searchPopup) savedResults(q string) []searchResult {
	names := make([]string, 0, len(p.sess.Queries))
	for n := range p.sess.Queries {
		names = append(names, n)
	}
	sort.Strings(names)
	items := make([]pickItem, 0, len(names)+len(p.favourites))
	for _, n := range names {
		items = append(items, pickItem{Label: n, Detail: "saved", Value: p.sess.ExpandJQL(p.sess.Queries[n])})
	}
	for _, f := range p.favourites {
		items = append(items, pickItem{Label: f.Name, Detail: "filter " + f.ID, Value: f.JQL})
	}
	var out []searchResult
	if q == "" {
		for i := range items {
			out = append(out, searchResult{query: &items[i]})
		}
		return out
	}
	src := make([]string, len(items))
	for i, it := range items {
		src[i] = it.Label + " " + it.Value
	}
	for _, m := range fuzzy.Find(q, src) {
		out = append(out, searchResult{query: &items[m.Index]})
	}
	return out
}

func (p *searchPopup) buildJQL() string { return SearchJQL(p.sess, p.input.Value()) }

// SearchJQL turns search input (with an optional mode prefix such as ">" or
// "#") into JQL, or "" if there is nothing to run. Saved-query input ("*")
// is resolved by name instead.
func SearchJQL(sess Session, input string) string {
	q := queryOf(input)
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(q)
	switch modeOf(input).name {
	case "jql":
		return q
	case "key":
		if sess.Project != "" && digitsRe.MatchString(q) {
			q = sess.Project + "-" + q
		}
		if issueKeyRe.MatchString(q) {
			return fmt.Sprintf("key = %s", strings.ToUpper(q))
		}
		return ""
	case "people":
		user := `"` + esc + `"`
		if strings.EqualFold(q, "me") {
			user = "currentUser()"
		}
		return and(sess.ProjectClause(), fmt.Sprintf("(assignee = %s OR reporter = %s)", user, user)) + " ORDER BY updated DESC"
	case "everywhere":
		var keys []string
		for _, c := range sess.Projects {
			if c.Key != "" {
				keys = append(keys, fmt.Sprintf("%q", c.Key))
			}
		}
		scope := ""
		if len(keys) > 0 {
			scope = "project in (" + strings.Join(keys, ", ") + ")"
		}
		return and(scope, textClause(q, esc)) + " ORDER BY updated DESC"
	case "saved":
		if jql, ok := sess.Queries[q]; ok {
			return sess.ExpandJQL(jql)
		}
		return ""
	default:
		return and(sess.ProjectClause(), textClause(q, esc)) + " ORDER BY updated DESC"
	}
}

// textClause matches free text, and the issue key itself when q looks like one.
func textClause(q, esc string) string {
	clause := fmt.Sprintf(`text ~ "%s*"`, esc)
	if strings.ContainsAny(q, " ") {
		clause = fmt.Sprintf(`text ~ "%s"`, esc)
	}
	if issueKeyRe.MatchString(q) {
		clause = fmt.Sprintf("(%s OR key = %s)", clause, strings.ToUpper(q))
	}
	return clause
}

func (p *searchPopup) runSearch() tea.Cmd {
	jql := p.buildJQL()
	if jql == "" {
		p.results = nil
		return nil
	}
	p.loading = true
	p.jql = jql
	seq, b := p.seq, p.backend
	return func() tea.Msg {
		res, err := b.Search(jql, 0, 50)
		if err != nil {
			return searchResultMsg{seq: seq, jql: jql, err: err}
		}
		return searchResultMsg{seq: seq, jql: jql, issues: res.Issues, total: res.Total}
	}
}

func (p *searchPopup) selected() *searchResult {
	if p.cursor < 0 || p.cursor >= len(p.results) {
		return nil
	}
	return &p.results[p.cursor]
}

func (p *searchPopup) Update(msg tea.Msg) (Popup, tea.Cmd) {
	switch msg := msg.(type) {
	case searchDebounceMsg:
		if msg.seq == p.seq {
			return p, p.runSearch()
		}
		return p, nil
	case searchResultMsg:
		if msg.seq != p.seq {
			return p, nil // stale: the query changed while this was in flight
		}
		p.loading = false
		p.err = msg.err
		p.total = msg.total
		p.results = p.results[:0]
		for _, iss := range msg.issues {
			p.results = append(p.results, searchResult{issue: iss})
		}
		return p, nil
	case favouritesMsg:
		if msg.err == nil {
			p.favourites = msg.filters
			if p.mode().name == "saved" {
				p.results = p.savedResults(p.query())
			}
		}
		return p, nil
	case tea.KeyPressMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p *searchPopup) handleKey(k tea.KeyPressMsg) (Popup, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c":
		return p, closePopup
	case "down", "ctrl+n", "ctrl+j":
		if p.cursor < len(p.results)-1 {
			p.cursor++
		}
		return p, nil
	case "up", "ctrl+p", "ctrl+k":
		if p.cursor > 0 {
			p.cursor--
		}
		return p, nil
	case "tab", "shift+tab":
		return p, p.cycleMode(k.String() == "tab")
	case "enter":
		return p, p.open()
	case "ctrl+o":
		if r := p.selected(); r != nil && r.issue != nil {
			return p, openInBrowser(p.sess, r.issue.Key)
		}
		return p, nil
	case "ctrl+y":
		if r := p.selected(); r != nil && r.issue != nil {
			return p, copyText(r.issue.Key, "copied "+r.issue.Key)
		}
		return p, nil
	case "ctrl+q":
		if p.jql == "" {
			return p, nil
		}
		spec := QueryList("Search: "+p.query(), p.jql)
		return p, tea.Sequence(closePopup, push(newListView(spec, p.backend, p.st)))
	case "ctrl+s":
		if p.jql == "" {
			return p, toast("nothing to save: run a server search first")
		}
		jql := p.jql
		return p, openPopup(newPrompt("save query as", "", func(name string) tea.Cmd {
			if err := SaveQuery(name, jql); err != nil {
				return toastErr(err)
			}
			return toast("saved query " + name)
		}))
	}

	var cmd tea.Cmd
	before := p.input.Value()
	p.input, cmd = p.input.Update(k)
	if p.input.Value() != before {
		return p, tea.Batch(cmd, p.onInput())
	}
	return p, cmd
}

func (p *searchPopup) cycleMode(forward bool) tea.Cmd {
	cur := 0
	for i, m := range searchModes {
		if m.name == p.mode().name {
			cur = i
		}
	}
	step := 1
	if !forward {
		step = len(searchModes) - 1
	}
	next := searchModes[(cur+step)%len(searchModes)]
	p.input.SetValue(next.prefix + p.query())
	p.input.CursorEnd()
	return p.onInput()
}

func (p *searchPopup) open() tea.Cmd {
	if p.mode().name == "key" {
		if jql := p.buildJQL(); jql != "" {
			key := strings.TrimPrefix(jql, "key = ")
			return tea.Sequence(closePopup, push(newDetailView(key, p.backend, p.st)))
		}
	}
	r := p.selected()
	switch {
	case r == nil:
		return nil
	case r.query != nil:
		spec := QueryList(r.query.Label, r.query.Value)
		return tea.Sequence(closePopup, push(newListView(spec, p.backend, p.st)))
	default:
		return tea.Sequence(closePopup, push(newDetailView(r.issue.Key, p.backend, p.st)))
	}
}

func (p *searchPopup) Render(st styles, width, height int) string {
	w := min(width*4/5, width-2)
	h := min(height*4/5, height-2)
	inner := w - st.Popup.GetHorizontalFrameSize()
	bodyH := max(h-6, 5)

	m := p.mode()
	badge := lipgloss.NewStyle().Background(st.Palette.Accent2).Foreground(st.Palette.Bg).Bold(true).Padding(0, 1).Render(m.name)
	p.input.SetWidth(inner - lipgloss.Width(badge) - 4)
	status := ""
	switch {
	case p.loading:
		status = st.Warn.Render(" ⟳")
	case p.err != nil:
		status = st.Error.Render(" ✖")
	}
	top := badge + " " + st.Accent.Render("❯ ") + p.input.View() + status

	listW := inner * 2 / 5
	if inner < 90 {
		listW = inner
	}
	list := p.renderList(st, listW, bodyH)
	body := list
	if listW < inner {
		var pv string
		if r := p.selected(); r != nil && r.issue != nil {
			pv = renderPreview(st, r.issue, inner-listW-1, bodyH)
		} else if r != nil && r.query != nil {
			pv = lipgloss.NewStyle().Width(inner - listW - 2).PaddingLeft(1).Foreground(st.Palette.Subtle).Render(r.query.Value)
		}
		sep := st.Muted.Render(strings.TrimRight(strings.Repeat("│\n", bodyH), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, list, sep, pv)
	}

	info := m.hint
	switch {
	case p.err != nil:
		info = st.Error.Render(fit(p.err.Error(), inner))
	case p.mode().name == "text" && p.query() == "":
		info = "recently viewed · " + m.hint
	case p.jql != "" && !p.loading:
		info = fmt.Sprintf("%d of %d · %s", len(p.results), p.total, p.jql)
	}
	content := top + "\n" + st.Muted.Render(strings.Repeat("─", inner)) + "\n" + body + "\n" + st.Muted.Render(fit(info, inner))
	return frame(st, "search", content, "enter open · tab mode · ctrl+o browser · ctrl+y copy · ctrl+q to list · ctrl+s save · esc", w)
}

func (p *searchPopup) renderList(st styles, width, height int) string {
	if len(p.results) == 0 {
		msg := "no results"
		if p.loading {
			msg = "searching…"
		} else if p.query() == "" && p.mode().name != "saved" {
			msg = "start typing"
		}
		return lipgloss.NewStyle().Width(width).Height(height).Render(st.Muted.Render("  " + msg))
	}
	start := 0
	if p.cursor >= height {
		start = p.cursor - height + 1
	}
	terms := strings.Fields(strings.ToLower(p.query()))
	var rows []string
	for i := start; i < len(p.results) && i < start+height; i++ {
		r := p.results[i]
		var line string
		if r.query != nil {
			line = st.Key.Render("★ ") + highlight(st, fit(r.query.Label, width-14), terms) + " " + st.Muted.Render(r.query.Detail)
		} else {
			f := r.issue.Fields
			line = st.typeIcon(f.IssueType) + " " + st.Accent.Render(fit(r.issue.Key, colKey)) + " " + highlight(st, fit(f.Summary, max(width-colKey-5, 5)), terms)
		}
		if i == p.cursor {
			line = st.RowSelected.Width(width).Render("▌" + ansiStrip(line))
		} else {
			line = " " + line
		}
		rows = append(rows, line)
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(rows, "\n"))
}

// highlight marks case-insensitive occurrences of terms in s.
func highlight(st styles, s string, terms []string) string {
	if len(terms) == 0 {
		return s
	}
	lower := strings.ToLower(s)
	if len(lower) != len(s) {
		return s // case folding changed byte offsets; skip highlighting
	}
	mark := make([]bool, len(s))
	for _, t := range terms {
		t = strings.Trim(t, `*"`)
		if t == "" {
			continue
		}
		for i := 0; ; {
			j := strings.Index(lower[i:], t)
			if j < 0 {
				break
			}
			for k := i + j; k < i+j+len(t) && k < len(mark); k++ {
				mark[k] = true
			}
			i += j + len(t)
		}
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && mark[j] == mark[i] {
			j++
		}
		if mark[i] {
			b.WriteString(st.Match.Render(s[i:j]))
		} else {
			b.WriteString(s[i:j])
		}
		i = j
	}
	return b.String()
}
