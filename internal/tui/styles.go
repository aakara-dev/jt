package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/aakara-dev/jt/internal/theme"
	"github.com/aakara-dev/jt/pkg/jira"
)

// styles are the TUI-specific styles derived from the active palette.
type styles struct {
	theme.Styles

	Subtle                                          lipgloss.Style
	App, StatusBar, StatusKey, StatusCtx, StatusDim lipgloss.Style
	Popup, PopupTitle                               lipgloss.Style
	Row, RowSelected, RowMarked                     lipgloss.Style
	Match                                           lipgloss.Style
	ToastInfo, ToastErr                             lipgloss.Style
}

func newStyles(name string) styles {
	t := theme.New(theme.Get(name))
	p := t.Palette
	s := lipgloss.NewStyle()
	return styles{
		Styles:      t,
		Subtle:      s.Foreground(p.Subtle),
		App:         s.Foreground(p.Text),
		StatusBar:   s.Background(p.Surface).Foreground(p.Subtle),
		StatusKey:   s.Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 1),
		StatusCtx:   s.Background(p.Accent2).Foreground(p.Bg).Bold(true).Padding(0, 1),
		StatusDim:   s.Background(p.Surface).Foreground(p.Muted).Padding(0, 1),
		Popup:       s.Border(lipgloss.RoundedBorder()).BorderForeground(p.Accent).Padding(0, 1),
		PopupTitle:  s.Foreground(p.Bg).Background(p.Accent).Bold(true).Padding(0, 1),
		Row:         s.Foreground(p.Text),
		RowSelected: s.Background(p.Overlay).Foreground(p.Text).Bold(true),
		RowMarked:   s.Foreground(p.Accent2),
		Match:       s.Foreground(p.Warn).Bold(true).Underline(true),
		ToastInfo:   s.Foreground(p.Bg).Background(p.Success).Padding(0, 1),
		ToastErr:    s.Foreground(p.Bg).Background(p.Error).Bold(true).Padding(0, 1),
	}
}

// statusBadge renders a status as a coloured pill.
func (st styles) statusBadge(s jira.IssueStatus) string {
	key := s.Category.Key
	if key == "" {
		key = s.Name
	}
	c := st.Palette.Status(key)
	return lipgloss.NewStyle().Foreground(c).Render("● " + s.Name)
}

// typeIcon is a one-cell glyph per issue type.
func (st styles) typeIcon(t jira.IssueType) string {
	p := st.Palette
	icon, c := "◆", p.Accent
	switch strings.ToLower(t.Name) {
	case "bug":
		icon, c = "✖", p.Error
	case "story":
		icon, c = "◼", p.Success
	case "epic":
		icon, c = "◈", p.Accent2
	case "task":
		icon, c = "✔", p.Info
	case "sub-task", "subtask":
		icon, c = "↳", p.Subtle
	}
	return lipgloss.NewStyle().Foreground(c).Render(icon)
}

// priorityIcon maps the default Jira priorities to arrows.
func (st styles) priorityIcon(name string) string {
	p := st.Palette
	icon, c := "·", p.Muted
	switch strings.ToLower(name) {
	case "blocker", "highest", "critical":
		icon, c = "⇈", p.Error
	case "high", "major":
		icon, c = "↑", p.Warn
	case "medium", "normal":
		icon, c = "=", p.Subtle
	case "low", "minor":
		icon, c = "↓", p.Info
	case "lowest", "trivial":
		icon, c = "⇊", p.Muted
	}
	return lipgloss.NewStyle().Foreground(c).Render(icon)
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-ansi.StringWidth(s))
}

// placeCenter overlays fg centered on bg (both multi-line strings).
func placeCenter(bg, fg string, width, height int) string {
	fw, fh := lipgloss.Size(fg)
	x, y := max((width-fw)/2, 0), max((height-fh)/3, 0)
	return placeAt(bg, fg, x, y)
}

// placeAt overlays fg on bg with its top-left corner at (x, y).
func placeAt(bg, fg string, x, y int) string {
	base := lipgloss.NewLayer(bg)
	top := lipgloss.NewLayer(fg).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(base, top).Render()
}
