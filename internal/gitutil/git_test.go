package gitutil

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueKey(t *testing.T) {
	cases := map[string]string{
		"feat/DATA-12-add-retry": "DATA-12",
		"fix/ops-7":              "OPS-7",
		"DATA_ENG-3":             "DATA_ENG-3",
		"main":                   "",
		"release/2024-10":        "",
		"user/aakara/DATA-9-wip": "DATA-9",
	}
	for branch, want := range cases {
		assert.Equal(t, want, IssueKey(branch), branch)
	}
}

func TestSwitchCreatesAndReuses(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		require.NoError(t, exec.Command("git", args...).Run())
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)

	created, err := Switch("feat/DATA-1-x", "")
	require.NoError(t, err)
	assert.True(t, created)
	cur, err := CurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "feat/DATA-1-x", cur)

	_, err = run("switch", "main")
	require.NoError(t, err)
	created, err = Switch("feat/DATA-1-x", "")
	require.NoError(t, err)
	assert.False(t, created, "existing branch is switched to, not recreated")
}

func TestNotARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	_, err := CurrentBranch()
	assert.Error(t, err)
}
