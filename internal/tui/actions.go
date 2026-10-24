package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/aakara-dev/jt/internal/branch"
	"github.com/aakara-dev/jt/pkg/browser"
	"github.com/aakara-dev/jt/pkg/jira"
)

func ansiStrip(s string) string { return ansi.Strip(s) }

func openInBrowser(s Session, key string) tea.Cmd {
	url := s.BrowseURL(key)
	return func() tea.Msg {
		if err := browser.Browse(url); err != nil {
			return toastMsg{text: err.Error(), err: true}
		}
		return toastMsg{text: "opened " + key}
	}
}

func copyText(text, done string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(text); err != nil {
			return toastMsg{text: "clipboard: " + err.Error(), err: true}
		}
		return toastMsg{text: done}
	}
}

func copyBranch(iss *jira.Issue) tea.Cmd {
	name := branch.Name(iss.Key, iss.Fields.Summary, iss.Fields.IssueType.Name)
	return copyText(name, "copied "+name)
}

// prompt is a single-line text input popup.
type prompt struct {
	title    string
	input    textinput.Model
	onSubmit func(string) tea.Cmd
}

func newPrompt(title, initial string, onSubmit func(string) tea.Cmd) *prompt {
	in := textinput.New()
	in.Prompt = "› "
	in.SetValue(initial)
	in.Focus()
	return &prompt{title: title, input: in, onSubmit: onSubmit}
}

func (p *prompt) Update(msg tea.Msg) (Popup, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc", "ctrl+c":
			return p, closePopup
		case "enter":
			if p.input.Value() == "" {
				return p, nil
			}
			return p, tea.Sequence(closePopup, p.onSubmit(p.input.Value()))
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd
}

func (p *prompt) Render(st styles, width, _ int) string {
	w := min(max(width/3, 40), width-4)
	p.input.SetWidth(w - st.Popup.GetHorizontalFrameSize() - 3)
	return frame(st, p.title, p.input.View(), "enter ok · esc cancel", w)
}
