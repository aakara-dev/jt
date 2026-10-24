package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Run starts the TUI and blocks until the user quits.
func Run(b Backend, opts Options) error {
	_, err := tea.NewProgram(NewApp(LoadSession(), b, opts)).Run()
	return err
}
