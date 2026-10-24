package standup

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aakara-dev/jt/pkg/jira"
)

func TestSinceClause(t *testing.T) {
	monday := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	tuesday := monday.AddDate(0, 0, 1)

	got, err := sinceClause("", monday)
	require.NoError(t, err)
	assert.Equal(t, "startOfDay(-3)", got)

	got, _ = sinceClause("", tuesday)
	assert.Equal(t, "startOfDay(-1)", got)

	got, _ = sinceClause("2d", tuesday)
	assert.Equal(t, "-2d", got)

	_, err = sinceClause("yesterday", tuesday)
	assert.Error(t, err)
}

func TestRender(t *testing.T) {
	iss := &jira.Issue{Key: "DATA-1"}
	iss.Fields.Summary = "Add retry"
	iss.Fields.Status.Name = "Done"
	sections := []*section{
		{title: "Yesterday", issues: []*jira.Issue{iss}},
		{title: "Today"},
	}

	var b strings.Builder
	render(&b, sections, "https://jira.example.com", false)
	assert.Equal(t, "**Yesterday**\n- DATA-1 Add retry — Done\n\n**Today**\n- nothing\n", b.String())

	b.Reset()
	render(&b, sections[:1], "https://jira.example.com", true)
	assert.Contains(t, b.String(), "[DATA-1](https://jira.example.com/browse/DATA-1)")
}
