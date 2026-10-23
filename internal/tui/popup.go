package tui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

// Popup is a floating window. While open it receives every key.
type Popup interface {
	Update(msg tea.Msg) (Popup, tea.Cmd)
	// Render returns the framed window sized for a screen of width x height.
	Render(st styles, width, height int) string
}

// frame draws a rounded window with a title tab and optional footer.
func frame(st styles, title, body, footer string, width int) string {
	box := st.Popup.Width(width)
	inner := width - box.GetHorizontalFrameSize()
	var b strings.Builder
	if title != "" {
		b.WriteString(st.PopupTitle.Render(title))
		b.WriteString("\n")
	}
	b.WriteString(body)
	if footer != "" {
		b.WriteString("\n")
		b.WriteString(st.Muted.Render(fit(footer, inner)))
	}
	return box.Render(b.String())
}

// --- picker -----------------------------------------------------------------

// pickItem is one row in a picker.
type pickItem struct {
	Label, Detail string
	Value         string
}

// picker is a fuzzy-filtered list (contexts, themes, saved queries, ...).
type picker struct {
	title    string
	items    []pickItem
	matches  []int
	cursor   int
	input    textinput.Model
	onSelect func(pickItem) tea.Cmd
}

func newPicker(title string, items []pickItem, onSelect func(pickItem) tea.Cmd) *picker {
	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "type to filter"
	in.Focus()
	p := &picker{title: title, items: items, input: in, onSelect: onSelect}
	p.filter()
	return p
}

// cursorTo moves the cursor to the item with this value, if present.
func (p *picker) cursorTo(value string) *picker {
	for i, idx := range p.matches {
		if p.items[idx].Value == value {
			p.cursor = i
		}
	}
	return p
}

func (p *picker) filter() {
	q := p.input.Value()
	p.matches = p.matches[:0]
	if q == "" {
		for i := range p.items {
			p.matches = append(p.matches, i)
		}
	} else {
		src := make([]string, len(p.items))
		for i, it := range p.items {
			src[i] = it.Label + " " + it.Detail
		}
		for _, m := range fuzzy.Find(q, src) {
			p.matches = append(p.matches, m.Index)
		}
	}
	if p.cursor >= len(p.matches) {
		p.cursor = max(len(p.matches)-1, 0)
	}
}

func (p *picker) Update(msg tea.Msg) (Popup, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc", "ctrl+c":
			return p, closePopup
		case "enter":
			if len(p.matches) == 0 {
				return p, nil
			}
			it := p.items[p.matches[p.cursor]]
			return p, tea.Sequence(closePopup, p.onSelect(it))
		case "down", "ctrl+n", "ctrl+j", "tab":
			if p.cursor < len(p.matches)-1 {
				p.cursor++
			}
			return p, nil
		case "up", "ctrl+p", "ctrl+k", "shift+tab":
			if p.cursor > 0 {
				p.cursor--
			}
			return p, nil
		}
	}
	var cmd tea.Cmd
	before := p.input.Value()
	p.input, cmd = p.input.Update(msg)
	if p.input.Value() != before {
		p.cursor = 0
		p.filter()
	}
	return p, cmd
}

func (p *picker) Render(st styles, width, height int) string {
	w := min(max(width/2, 40), width-4, 80)
	inner := w - st.Popup.GetHorizontalFrameSize()
	p.input.SetWidth(inner - 3)

	maxRows := min(len(p.matches), max(height/2, 5))
	start := 0
	if p.cursor >= maxRows {
		start = p.cursor - maxRows + 1
	}
	var rows []string
	for i := start; i < start+maxRows && i < len(p.matches); i++ {
		it := p.items[p.matches[i]]
		label := fit(it.Label, inner/2)
		line := label + " " + st.Muted.Render(fit(it.Detail, inner-inner/2-3))
		if i == p.cursor {
			line = st.RowSelected.Width(inner).Render("▌" + label + " " + fit(it.Detail, inner-inner/2-3))
		} else {
			line = " " + line
		}
		rows = append(rows, line)
	}
	if len(rows) == 0 {
		rows = append(rows, st.Muted.Render("  no matches"))
	}
	body := p.input.View() + "\n" + strings.Join(rows, "\n")
	return frame(st, p.title, body, "↑↓ move · enter select · esc close", w)
}

// --- which-key --------------------------------------------------------------

// whichKey is the <space> leader menu.
type whichKey struct{}

func (w whichKey) Update(msg tea.Msg) (Popup, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return w, nil
	}
	key := k.String()
	if key == "esc" || key == "space" || key == "ctrl+c" {
		return w, closePopup
	}
	for _, e := range leaderMenu {
		if e.Key == key {
			return w, tea.Sequence(closePopup, runCommand(e.Cmd))
		}
	}
	return w, nil
}

func (whichKey) Render(st styles, width, _ int) string {
	groups := map[string][]leaderEntry{}
	var order []string
	for _, e := range leaderMenu {
		if _, ok := groups[e.Group]; !ok {
			order = append(order, e.Group)
		}
		groups[e.Group] = append(groups[e.Group], e)
	}
	colW := 24
	var cols []string
	for _, g := range order {
		lines := []string{st.Accent2.Bold(true).Render(g)}
		for _, e := range groups[g] {
			lines = append(lines, st.Key.Render(fit(e.Key, 2))+" "+st.Muted.Render("→")+" "+st.Value.Render(fit(e.Desc, colW-5)))
		}
		cols = append(cols, lipgloss.NewStyle().Width(colW).Render(strings.Join(lines, "\n")))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	w := min(lipgloss.Width(body)+st.Popup.GetHorizontalFrameSize(), width-2)
	return frame(st, "<leader>", body, "press a key · esc close", w)
}

// --- help -------------------------------------------------------------------

// helpPopup lists global keys, the active view's keys and ":" commands.
type helpPopup struct {
	view []Hint
}

func (h helpPopup) Update(msg tea.Msg) (Popup, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc", "q", "?", "enter", "ctrl+c":
			return h, closePopup
		}
	}
	return h, nil
}

func (h helpPopup) Render(st styles, width, _ int) string {
	section := func(title string, hints []Hint) string {
		kw := 0
		for _, x := range hints {
			kw = max(kw, lipgloss.Width(x.Key))
		}
		lines := []string{st.Accent2.Bold(true).Render(title)}
		for _, x := range hints {
			lines = append(lines, st.Key.Render(fit(x.Key, kw))+"  "+st.Value.Render(x.Desc))
		}
		return strings.Join(lines, "\n")
	}
	left := section("Global", globalHints)
	right := section("This view", h.view) + "\n\n" + section("Commands", commandHelp)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right)
	w := min(lipgloss.Width(body)+st.Popup.GetHorizontalFrameSize(), width-2)
	return frame(st, "help", body, "esc close", w)
}

// --- command line -----------------------------------------------------------

// cmdline is the ":" prompt, drawn as a small floating window like
// LazyVim's noice cmdline, with fuzzy completion.
type cmdline struct {
	input       textinput.Model
	completions func(string) []string
	matches     []string
	cursor      int
	history     *[]string
	histPos     int
}

func newCmdline(complete func(string) []string, history *[]string) *cmdline {
	in := textinput.New()
	in.Prompt = ": "
	in.Focus()
	c := &cmdline{input: in, completions: complete, history: history, histPos: len(*history)}
	c.refresh()
	return c
}

func (c *cmdline) refresh() {
	c.matches = c.completions(c.input.Value())
	c.cursor = -1
}

func (c *cmdline) Update(msg tea.Msg) (Popup, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc", "ctrl+c":
			return c, closePopup
		case "enter":
			line := strings.TrimSpace(c.input.Value())
			if c.cursor >= 0 && c.cursor < len(c.matches) {
				line = c.matches[c.cursor]
			}
			if line == "" {
				return c, closePopup
			}
			*c.history = append(*c.history, line)
			return c, tea.Sequence(closePopup, runCommand(line))
		case "tab", "ctrl+n", "down":
			if len(c.matches) > 0 {
				c.cursor = (c.cursor + 1) % len(c.matches)
			}
			return c, nil
		case "shift+tab", "ctrl+p":
			if len(c.matches) > 0 {
				c.cursor = (c.cursor - 1 + len(c.matches)) % len(c.matches)
			}
			return c, nil
		case "up":
			if c.histPos > 0 {
				c.histPos--
				c.input.SetValue((*c.history)[c.histPos])
				c.input.CursorEnd()
				c.refresh()
			}
			return c, nil
		}
	}
	var cmd tea.Cmd
	before := c.input.Value()
	c.input, cmd = c.input.Update(msg)
	if c.input.Value() != before {
		c.refresh()
	}
	return c, cmd
}

func (c *cmdline) Render(st styles, width, _ int) string {
	w := min(max(width/2, 50), width-4, 90)
	inner := w - st.Popup.GetHorizontalFrameSize()
	c.input.SetWidth(inner - 3)
	body := c.input.View()
	n := min(len(c.matches), 8)
	for i := 0; i < n; i++ {
		m := fit(c.matches[i], inner-2)
		if i == c.cursor {
			body += "\n" + st.RowSelected.Width(inner).Render("▌"+m)
		} else {
			body += "\n " + st.Muted.Render(m)
		}
	}
	return frame(st, "cmdline", body, "tab complete · ↑ history · enter run", w)
}

// fuzzyFilter returns candidates matching q (prefix matches first).
func fuzzyFilter(q string, candidates []string) []string {
	if q == "" {
		out := append([]string(nil), candidates...)
		sort.Strings(out)
		return out
	}
	var prefix, rest []string
	seen := map[string]bool{}
	for _, c := range candidates {
		if strings.HasPrefix(c, q) && c != q {
			prefix = append(prefix, c)
			seen[c] = true
		}
	}
	sort.Strings(prefix)
	if len(prefix) > 0 {
		return prefix
	}
	for _, m := range fuzzy.Find(q, candidates) {
		if !seen[m.Str] && m.Str != q {
			rest = append(rest, m.Str)
		}
	}
	return append(prefix, rest...)
}
