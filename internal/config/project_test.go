package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleConfig = `# my jt config
server: https://jira.example.com
login: me
project:
  key: DEF
  type: classic
board:
  id: 1
  name: Default board
  type: scrum
issue:
  types:
    - name: Task
current_project: data
projects:
  data:
    project:
      key: DATA
    board:
      id: 142
      name: Data board
  ops:
    project:
      key: OPS
    board: ""
aliases:
  mine: issue list -a me # keep this comment
`

func writeSample(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(p, []byte(sampleConfig), 0o600))
	return p
}

func newViper(t *testing.T, path string) *viper.Viper {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(path)
	require.NoError(t, v.ReadInConfig())
	return v
}

func TestApplyProjectReplacesProjectKeys(t *testing.T) {
	v := newViper(t, writeSample(t))

	require.NoError(t, ApplyProject(v, "data"))

	assert.Equal(t, "DATA", v.GetString("project.key"))
	assert.Equal(t, "", v.GetString("project.type"), "context keys replace, not merge")
	assert.Equal(t, 142, v.GetInt("board.id"))
	assert.Equal(t, "", v.GetString("board.type"))
	assert.Equal(t, "https://jira.example.com", v.GetString("server"), "server-wide keys untouched")
	assert.Len(t, v.Get("issue.types"), 1, "keys absent from the context keep top-level values")
	assert.Equal(t, "data", v.GetString(KeyCurrentProject))
}

func TestApplyProjectEmptyBoard(t *testing.T) {
	v := newViper(t, writeSample(t))

	require.NoError(t, ApplyProject(v, "ops"))

	assert.Equal(t, "OPS", v.GetString("project.key"))
	assert.Equal(t, "", v.GetString("board.id"))
}

func TestApplyProjectUnknown(t *testing.T) {
	v := newViper(t, writeSample(t))

	assert.ErrorContains(t, ApplyProject(v, "nope"), `project "nope" not found`)
	assert.NoError(t, ApplyProject(v, ""))
	assert.Equal(t, "DEF", v.GetString("project.key"))
}

func TestFileUseProjectAndSwapBack(t *testing.T) {
	p := writeSample(t)
	f, err := LoadFile(p)
	require.NoError(t, err)

	name, err := f.UseProject("ops")
	require.NoError(t, err)
	assert.Equal(t, "ops", name)
	require.NoError(t, f.Save())

	f, err = LoadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "ops", f.GetString(KeyCurrentProject))
	assert.Equal(t, "data", f.GetString(KeyPreviousProject))

	name, err = f.UseProject("-")
	require.NoError(t, err)
	assert.Equal(t, "data", name)
	assert.Equal(t, "ops", f.GetString(KeyPreviousProject))

	_, err = f.UseProject("missing")
	assert.Error(t, err)
}

func TestFileSavePreservesComments(t *testing.T) {
	p := writeSample(t)
	f, err := LoadFile(p)
	require.NoError(t, err)

	require.NoError(t, f.Set("issue list -s Blocked", KeyAliases, "blocked"))
	require.NoError(t, f.Save())

	out, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Contains(t, string(out), "# my jt config")
	assert.Contains(t, string(out), "# keep this comment")
	assert.Contains(t, string(out), "blocked: issue list -s Blocked")

	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestFileProjectsAndRenameDelete(t *testing.T) {
	f, err := LoadFile(writeSample(t))
	require.NoError(t, err)

	ctxs := f.Projects()
	require.Len(t, ctxs, 2)
	assert.Equal(t, SavedProject{Name: "data", Key: "DATA", Board: "Data board", BoardID: "142", Current: true}, ctxs[0])
	assert.Equal(t, "OPS", ctxs[1].Key)

	require.NoError(t, f.RenameProject("data", "analytics"))
	assert.Equal(t, "analytics", f.GetString(KeyCurrentProject))
	assert.Error(t, f.RenameProject("ops", "analytics"))
	assert.Error(t, f.RenameProject("ops", "bad name"))

	require.NoError(t, f.DeleteProject("analytics"))
	assert.Equal(t, "", f.GetString(KeyCurrentProject))
	assert.Error(t, f.DeleteProject("analytics"))
}

func TestLoadFileMissing(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "none.yml"))
	assert.ErrorIs(t, err, ErrNoConfig)
}

func TestValidateProjectName(t *testing.T) {
	assert.NoError(t, ValidateProjectName("data-eng.v2"))
	assert.Error(t, ValidateProjectName("-"))
	assert.Error(t, ValidateProjectName("has space"))
	assert.Error(t, ValidateProjectName(""))
}

func TestFileResolve(t *testing.T) {
	f, err := LoadFile(writeSample(t))
	require.NoError(t, err)

	assert.Equal(t, "data", f.Resolve("data"))
	assert.Equal(t, "ops", f.Resolve("ops"))
	assert.Equal(t, "ops", f.Resolve("OPS"), "Jira key resolves to the saved project")
	assert.Equal(t, "", f.Resolve("NOPE"))
	assert.Equal(t, "", f.Resolve(""))
}
