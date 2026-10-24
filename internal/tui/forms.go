package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/shlex"

	"github.com/aakara-dev/jt/pkg/env"
)

// --- $EDITOR ----------------------------------------------------------------

// editorDoneMsg carries text written in $EDITOR back to whoever asked.
type editorDoneMsg struct {
	id   int
	text string
	err  error
}

var editorSeq int

// editInEditor suspends the TUI, opens initial in $JT_EDITOR/$VISUAL/$EDITOR
// (vi by default) and delivers the saved text as editorDoneMsg{id}.
func editInEditor(initial, suffix string) (int, tea.Cmd) {
	editorSeq++
	id := editorSeq
	f, err := os.CreateTemp("", "jt-*"+suffix)
	if err != nil {
		return id, func() tea.Msg { return editorDoneMsg{id: id, err: err} }
	}
	path := f.Name()
	_, _ = f.WriteString(initial)
	_ = f.Close()

	editor := env.Get("EDITOR")
	for _, e := range []string{"VISUAL", "EDITOR"} {
		if editor == "" {
			editor = os.Getenv(e)
		}
	}
	if editor == "" {
		editor = "vi"
	}
	args, err := shlex.Split(editor)
	if err != nil || len(args) == 0 {
		args = []string{"vi"}
	}
	cmd := exec.Command(args[0], append(args[1:], path)...) //nolint:gosec // the user's own editor
	return id, tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer func() { _ = os.Remove(path) }()
		if err != nil {
			return editorDoneMsg{id: id, err: err}
		}
		data, rerr := os.ReadFile(path)
		return editorDoneMsg{id: id, text: string(data), err: rerr}
	})
}

// --- async picker -------------------------------------------------------------

type asyncItemsMsg struct {
	seq   int
	items []pickItem
	err   error
}

type asyncDebounceMsg struct{ seq int }

// asyncPicker is a picker whose items come from a server-side search
// (e.g. assignable users), debounced as the user types.
type asyncPicker struct {
	*picker
	source  func(q string) ([]pickItem, error)
	seq     int
	loading bool
	err     error
}

func newAsyncPicker(title string, fixed []pickItem, source func(string) ([]pickItem, error), onSelect func(pickItem) tea.Cmd) (*asyncPicker, tea.Cmd) {
	p := &asyncPicker{picker: newPicker(title, fixed, onSelect), source: source}
	p.input.Placeholder = "type to search"
	return p, p.query()
}

func (p *asyncPicker) query() tea.Cmd {
	p.seq++
	seq := p.seq
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg { return asyncDebounceMsg{seq} })
}

func (p *asyncPicker) Update(msg tea.Msg) (Popup, tea.Cmd) {
	switch msg := msg.(type) {
	case asyncDebounceMsg:
		if msg.seq != p.seq {
			return p, nil
		}
		p.loading = true
		q, src, seq := p.input.Value(), p.source, p.seq
		return p, func() tea.Msg {
			items, err := src(q)
			return asyncItemsMsg{seq, items, err}
		}
	case asyncItemsMsg:
		if msg.seq != p.seq {
			return p, nil
		}
		p.loading, p.err = false, msg.err
		p.items = msg.items
		p.input.SetValue(p.input.Value()) // keep the query
		p.matches = p.matches[:0]
		for i := range p.items {
			p.matches = append(p.matches, i)
		}
		p.cursor = 0
		return p, nil
	case tea.KeyPressMsg:
		before := p.input.Value()
		_, cmd := p.picker.Update(msg)
		if p.input.Value() != before {
			return p, tea.Batch(cmd, p.query())
		}
		return p, cmd
	}
	return p, nil
}

func (p *asyncPicker) Render(st styles, width, height int) string {
	title := p.title
	switch {
	case p.loading:
		p.title = title + " ⟳"
	case p.err != nil:
		p.title = title + " ✖ " + p.err.Error()
	}
	out := p.picker.Render(st, width, height)
	p.title = title
	return out
}

// --- text area ----------------------------------------------------------------

// textPopup is a multi-line editor (comments, descriptions). ctrl+s submits,
// ctrl+e continues in $EDITOR.
type textPopup struct {
	title    string
	area     textarea.Model
	onSubmit func(string) tea.Cmd
	editorID int
}

func newTextPopup(title, initial string, onSubmit func(string) tea.Cmd) (*textPopup, tea.Cmd) {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Placeholder = "markdown supported"
	ta.SetValue(initial)
	focus := ta.Focus()
	return &textPopup{title: title, area: ta, onSubmit: onSubmit}, focus
}

func (p *textPopup) Update(msg tea.Msg) (Popup, tea.Cmd) {
	switch msg := msg.(type) {
	case editorDoneMsg:
		if msg.id != p.editorID {
			return p, nil
		}
		if msg.err != nil {
			return p, toastErr(msg.err)
		}
		p.area.SetValue(strings.TrimRight(msg.text, "\n"))
		return p, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return p, closePopup
		case "ctrl+s":
			text := strings.TrimSpace(p.area.Value())
			if text == "" {
				return p, nil
			}
			return p, tea.Sequence(closePopup, p.onSubmit(text))
		case "ctrl+e":
			id, cmd := editInEditor(p.area.Value(), ".md")
			p.editorID = id
			return p, cmd
		}
	}
	var cmd tea.Cmd
	p.area, cmd = p.area.Update(msg)
	return p, cmd
}

func (p *textPopup) Render(st styles, width, height int) string {
	w := min(max(width*3/5, 50), width-4)
	p.area.SetWidth(w - st.Popup.GetHorizontalFrameSize())
	p.area.SetHeight(max(min(height/2, 16), 5))
	return frame(st, p.title, p.area.View(), "ctrl+s save · ctrl+e $EDITOR · esc cancel", w)
}

// --- confirm ------------------------------------------------------------------

// confirmPopup asks a yes/no question; only "y" confirms.
type confirmPopup struct {
	title, question string
	onYes           tea.Cmd
}

func (p confirmPopup) Update(msg tea.Msg) (Popup, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "y", "Y":
			return p, tea.Sequence(closePopup, p.onYes)
		case "n", "N", "esc", "q", "ctrl+c", "enter":
			return p, closePopup
		}
	}
	return p, nil
}

func (p confirmPopup) Render(st styles, width, _ int) string {
	w := min(max(lipgloss.Width(p.question)+6, 40), width-4)
	body := st.Warn.Render(p.question) + "\n\n" + st.Key.Render("y") + st.Muted.Render(" yes   ") + st.Key.Render("n") + st.Muted.Render(" no")
	return frame(st, p.title, body, "", w)
}

// --- form ---------------------------------------------------------------------

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldChoice
	fieldArea
)

// formField is one row in a form.
type formField struct {
	label    string
	kind     fieldKind
	required bool
	input    textinput.Model // fieldText
	area     textarea.Model  // fieldArea
	choices  []string        // fieldChoice
	choice   int
}

func (f *formField) value() string {
	switch f.kind {
	case fieldChoice:
		if len(f.choices) == 0 {
			return ""
		}
		return f.choices[f.choice]
	case fieldArea:
		return strings.TrimSpace(f.area.Value())
	default:
		return strings.TrimSpace(f.input.Value())
	}
}

// formPopup is a small multi-field form (create issue). tab/shift+tab move
// between fields, ←/→ cycle choices, ctrl+s submits, ctrl+e edits the
// multi-line field in $EDITOR.
type formPopup struct {
	title    string
	fields   []*formField
	focus    int
	err      string
	onSubmit func(map[string]string) tea.Cmd
	editorID int
}

func newForm(title string, onSubmit func(map[string]string) tea.Cmd) *formPopup {
	return &formPopup{title: title, onSubmit: onSubmit}
}

func (f *formPopup) text(label, value string, required bool) *formPopup {
	in := textinput.New()
	in.Prompt = ""
	in.SetValue(value)
	f.fields = append(f.fields, &formField{label: label, kind: fieldText, required: required, input: in})
	return f
}

func (f *formPopup) choice(label string, choices []string, selected string) *formPopup {
	ff := &formField{label: label, kind: fieldChoice, choices: choices}
	for i, c := range choices {
		if strings.EqualFold(c, selected) {
			ff.choice = i
		}
	}
	f.fields = append(f.fields, ff)
	return f
}

func (f *formPopup) area(label, value string) *formPopup {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Placeholder = "markdown · ctrl+e opens $EDITOR"
	ta.SetValue(value)
	f.fields = append(f.fields, &formField{label: label, kind: fieldArea, area: ta})
	return f
}

// start focuses the first field.
func (f *formPopup) start() (*formPopup, tea.Cmd) {
	return f, f.setFocus(0)
}

func (f *formPopup) setFocus(i int) tea.Cmd {
	for _, ff := range f.fields {
		ff.input.Blur()
		ff.area.Blur()
	}
	f.focus = (i + len(f.fields)) % len(f.fields)
	switch ff := f.fields[f.focus]; ff.kind {
	case fieldText:
		return ff.input.Focus()
	case fieldArea:
		return ff.area.Focus()
	}
	return nil
}

func (f *formPopup) values() map[string]string {
	out := make(map[string]string, len(f.fields))
	for _, ff := range f.fields {
		out[ff.label] = ff.value()
	}
	return out
}

func (f *formPopup) Update(msg tea.Msg) (Popup, tea.Cmd) {
	cur := f.fields[f.focus]
	switch msg := msg.(type) {
	case editorDoneMsg:
		if msg.id == f.editorID && msg.err == nil {
			for _, ff := range f.fields {
				if ff.kind == fieldArea {
					ff.area.SetValue(strings.TrimRight(msg.text, "\n"))
				}
			}
		}
		return f, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return f, closePopup
		case "tab", "down":
			if msg.String() == "down" && cur.kind == fieldArea {
				break
			}
			return f, f.setFocus(f.focus + 1)
		case "shift+tab", "up":
			if msg.String() == "up" && cur.kind == fieldArea {
				break
			}
			return f, f.setFocus(f.focus - 1)
		case "left", "h":
			if cur.kind == fieldChoice && len(cur.choices) > 0 {
				cur.choice = (cur.choice - 1 + len(cur.choices)) % len(cur.choices)
				return f, nil
			}
		case "right", "l", "space":
			if cur.kind == fieldChoice && len(cur.choices) > 0 {
				cur.choice = (cur.choice + 1) % len(cur.choices)
				return f, nil
			}
		case "ctrl+e":
			for _, ff := range f.fields {
				if ff.kind == fieldArea {
					id, cmd := editInEditor(ff.area.Value(), ".md")
					f.editorID = id
					return f, cmd
				}
			}
		case "ctrl+s", "enter":
			if msg.String() == "enter" && cur.kind == fieldArea {
				break
			}
			for _, ff := range f.fields {
				if ff.required && ff.value() == "" {
					f.err = ff.label + " is required"
					return f, nil
				}
			}
			return f, tea.Sequence(closePopup, f.onSubmit(f.values()))
		}
	}

	var cmd tea.Cmd
	switch cur.kind {
	case fieldText:
		cur.input, cmd = cur.input.Update(msg)
	case fieldArea:
		cur.area, cmd = cur.area.Update(msg)
	}
	return f, cmd
}

func (f *formPopup) Render(st styles, width, height int) string {
	w := min(max(width*3/5, 60), width-4)
	inner := w - st.Popup.GetHorizontalFrameSize()
	labelW := 0
	for _, ff := range f.fields {
		labelW = max(labelW, len(ff.label))
	}
	fieldW := inner - labelW - 3

	rows := make([]string, 0, len(f.fields))
	for i, ff := range f.fields {
		label := ff.label
		if ff.required {
			label += "*"
		}
		ls := st.Muted
		if i == f.focus {
			ls = st.Key
		}
		var val string
		switch ff.kind {
		case fieldText:
			ff.input.SetWidth(fieldW)
			val = ff.input.View()
		case fieldChoice:
			var parts []string
			for j, c := range ff.choices {
				if j == ff.choice {
					parts = append(parts, st.Selected.Render("["+c+"]"))
				} else {
					parts = append(parts, st.Muted.Render(c))
				}
			}
			val = fit(strings.Join(parts, " "), fieldW)
		case fieldArea:
			ff.area.SetWidth(fieldW)
			ff.area.SetHeight(max(min(height/4, 8), 3))
			val = ff.area.View()
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, ls.Render(fit(label, labelW+2)), " ", val)
		rows = append(rows, row)
	}
	body := strings.Join(rows, "\n")
	if f.err != "" {
		body += "\n" + st.Error.Render(f.err)
	}
	return frame(st, f.title, body, "tab next · ←/→ choose · ctrl+e $EDITOR · ctrl+s create · esc cancel", w)
}

// parseLabels splits "a, b c" into labels.
func parseLabels(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

// errorf is fmt.Errorf for toasts.
func errorf(format string, a ...any) tea.Cmd { return toastErr(fmt.Errorf(format, a...)) }
