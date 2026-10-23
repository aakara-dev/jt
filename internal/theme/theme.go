// Package theme holds jt's colour palettes and the lipgloss styles built on
// them. CLI output and the TUI both render through this package so that a
// `theme:` change in the config restyles everything at once.
package theme

import (
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// Palette is a set of semantic colours.
type Palette struct {
	Name string

	Bg, Surface, Overlay color.Color // background layers: base, panels, popups
	Text, Subtle, Muted  color.Color // foreground emphasis levels
	Accent, Accent2      color.Color // primary highlight, secondary highlight
	Success, Warn, Error color.Color
	Info                 color.Color

	// Jira status categories.
	StatusTodo, StatusProgress, StatusDone color.Color
}

var palettes = map[string]Palette{
	"tokyonight": {
		Name: "tokyonight",
		Bg:   lipgloss.Color("#1a1b26"), Surface: lipgloss.Color("#24283b"), Overlay: lipgloss.Color("#2f334d"),
		Text: lipgloss.Color("#c0caf5"), Subtle: lipgloss.Color("#a9b1d6"), Muted: lipgloss.Color("#565f89"),
		Accent: lipgloss.Color("#7aa2f7"), Accent2: lipgloss.Color("#bb9af7"),
		Success: lipgloss.Color("#9ece6a"), Warn: lipgloss.Color("#e0af68"), Error: lipgloss.Color("#f7768e"),
		Info:       lipgloss.Color("#7dcfff"),
		StatusTodo: lipgloss.Color("#a9b1d6"), StatusProgress: lipgloss.Color("#e0af68"), StatusDone: lipgloss.Color("#9ece6a"),
	},
	"catppuccin": {
		Name: "catppuccin",
		Bg:   lipgloss.Color("#1e1e2e"), Surface: lipgloss.Color("#313244"), Overlay: lipgloss.Color("#45475a"),
		Text: lipgloss.Color("#cdd6f4"), Subtle: lipgloss.Color("#bac2de"), Muted: lipgloss.Color("#6c7086"),
		Accent: lipgloss.Color("#89b4fa"), Accent2: lipgloss.Color("#cba6f7"),
		Success: lipgloss.Color("#a6e3a1"), Warn: lipgloss.Color("#f9e2af"), Error: lipgloss.Color("#f38ba8"),
		Info:       lipgloss.Color("#94e2d5"),
		StatusTodo: lipgloss.Color("#bac2de"), StatusProgress: lipgloss.Color("#fab387"), StatusDone: lipgloss.Color("#a6e3a1"),
	},
	"gruvbox": {
		Name: "gruvbox",
		Bg:   lipgloss.Color("#282828"), Surface: lipgloss.Color("#3c3836"), Overlay: lipgloss.Color("#504945"),
		Text: lipgloss.Color("#ebdbb2"), Subtle: lipgloss.Color("#d5c4a1"), Muted: lipgloss.Color("#928374"),
		Accent: lipgloss.Color("#83a598"), Accent2: lipgloss.Color("#d3869b"),
		Success: lipgloss.Color("#b8bb26"), Warn: lipgloss.Color("#fabd2f"), Error: lipgloss.Color("#fb4934"),
		Info:       lipgloss.Color("#8ec07c"),
		StatusTodo: lipgloss.Color("#d5c4a1"), StatusProgress: lipgloss.Color("#fe8019"), StatusDone: lipgloss.Color("#b8bb26"),
	},
	"nord": {
		Name: "nord",
		Bg:   lipgloss.Color("#2e3440"), Surface: lipgloss.Color("#3b4252"), Overlay: lipgloss.Color("#434c5e"),
		Text: lipgloss.Color("#eceff4"), Subtle: lipgloss.Color("#d8dee9"), Muted: lipgloss.Color("#616e88"),
		Accent: lipgloss.Color("#88c0d0"), Accent2: lipgloss.Color("#b48ead"),
		Success: lipgloss.Color("#a3be8c"), Warn: lipgloss.Color("#ebcb8b"), Error: lipgloss.Color("#bf616a"),
		Info:       lipgloss.Color("#81a1c1"),
		StatusTodo: lipgloss.Color("#d8dee9"), StatusProgress: lipgloss.Color("#d08770"), StatusDone: lipgloss.Color("#a3be8c"),
	},
}

// Default is the palette used when none is configured.
const Default = "tokyonight"

// Names lists the built-in palettes.
func Names() []string {
	out := make([]string, 0, len(palettes))
	for n := range palettes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Get returns the named palette, falling back to Default.
func Get(name string) Palette {
	if p, ok := palettes[strings.ToLower(name)]; ok {
		return p
	}
	return palettes[Default]
}

// Styles are the shared lipgloss styles derived from a palette.
type Styles struct {
	Palette Palette

	Title, Header, Key, Value lipgloss.Style
	Muted, Accent, Accent2    lipgloss.Style
	Success, Warn, Error      lipgloss.Style
	Selected                  lipgloss.Style
}

// New builds styles for a palette.
func New(p Palette) Styles {
	s := lipgloss.NewStyle()
	return Styles{
		Palette:  p,
		Title:    s.Foreground(p.Accent).Bold(true),
		Header:   s.Foreground(p.Muted).Bold(true),
		Key:      s.Foreground(p.Accent2).Bold(true),
		Value:    s.Foreground(p.Text),
		Muted:    s.Foreground(p.Muted),
		Accent:   s.Foreground(p.Accent),
		Accent2:  s.Foreground(p.Accent2),
		Success:  s.Foreground(p.Success),
		Warn:     s.Foreground(p.Warn),
		Error:    s.Foreground(p.Error),
		Selected: s.Foreground(p.Success).Bold(true),
	}
}

// Status picks a colour for a Jira status by its category key
// ("new", "indeterminate", "done") or, failing that, by name.
func (p Palette) Status(categoryOrName string) color.Color {
	switch strings.ToLower(categoryOrName) {
	case "done", "closed", "resolved":
		return p.StatusDone
	case "indeterminate", "in progress", "in review", "review":
		return p.StatusProgress
	default:
		return p.StatusTodo
	}
}
