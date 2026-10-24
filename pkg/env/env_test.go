package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	t.Setenv("JT_FOO", "")
	t.Setenv("JIRA_FOO", "legacy")
	assert.Equal(t, "legacy", Get("FOO"))

	t.Setenv("JT_FOO", "new")
	assert.Equal(t, "new", Get("FOO"))

	assert.Equal(t, "", Get("DOES_NOT_EXIST_XYZ"))
}
