package theme

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFallsBackToDefault(t *testing.T) {
	assert.Equal(t, "nord", Get("NORD").Name)
	assert.Equal(t, Default, Get("does-not-exist").Name)
	assert.Equal(t, []string{"catppuccin", "gruvbox", "nord", "tokyonight"}, Names())
}

func TestStatusColour(t *testing.T) {
	p := Get(Default)
	assert.Equal(t, p.StatusDone, p.Status("done"))
	assert.Equal(t, p.StatusProgress, p.Status("indeterminate"))
	assert.Equal(t, p.StatusTodo, p.Status("new"))
}

func TestTablePlainWhenNotATerminal(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Table(&buf, New(Get(Default)), []string{"A", "B"}, [][]string{{"x", "y"}}, 0))
	assert.NotContains(t, buf.String(), "\x1b[", "no escape codes for non-TTY writers")
	assert.Contains(t, buf.String(), "x  y")
}
