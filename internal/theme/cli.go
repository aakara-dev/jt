package theme

import (
	"io"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/viper"
)

// Active returns styles for the configured `theme:`.
func Active() Styles {
	return New(Get(viper.GetString("theme")))
}

// Table renders a borderless table to w. Row `highlight` (or -1 for none)
// is drawn in the selected style. Colours are downsampled to what w
// supports, so piped output stays plain.
func Table(w io.Writer, s Styles, headers []string, rows [][]string, highlight int) error {
	const gap = 2
	t := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return s.Header.PaddingRight(gap)
			case row == highlight:
				return s.Selected.PaddingRight(gap)
			default:
				return s.Value.PaddingRight(gap)
			}
		})
	_, err := lipgloss.Fprintln(w, t.Render())
	return err
}

// Fprintln prints styled text, downsampling colours for w.
func Fprintln(w io.Writer, v ...any) {
	_, _ = lipgloss.Fprintln(w, v...)
}
