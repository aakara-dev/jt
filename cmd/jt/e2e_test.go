package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJira is a tiny Jira Server stand-in recording every request.
type fakeJira struct {
	mu       sync.Mutex
	requests []string
}

func (f *fakeJira) handler(t *testing.T) http.Handler {
	issue := func(key, summary, status, category string) map[string]any {
		return map[string]any{"key": key, "fields": map[string]any{
			"summary":   summary,
			"status":    map[string]any{"id": "3", "name": status, "statusCategory": map[string]any{"key": category}},
			"issueType": map[string]any{"name": "Story"},
			"priority":  map[string]any{"name": "High"},
			"assignee":  map[string]any{"displayName": "Ada Lovelace"},
			"created":   "2026-10-20T10:00:00.000+0000",
			"updated":   "2026-10-23T10:00:00.000+0000",
		}}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		f.mu.Unlock()

		user, pass, ok := r.BasicAuth()
		if !ok || user != "aakara" || pass != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/api/2/search":
			jql := r.URL.Query().Get("jql")
			issues := []any{issue("DATA-12", "Add retry to the loader", "In Progress", "indeterminate")}
			if strings.Contains(jql, "status = Blocked") {
				issues = nil
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"startAt": 0, "total": len(issues), "issues": issues})
		case strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/"):
			key := strings.TrimPrefix(r.URL.Path, "/rest/api/2/issue/")
			_ = json.NewEncoder(w).Encode(issue(key, "Add retry to the loader", "In Progress", "indeterminate"))
		default:
			t.Logf("unhandled %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (f *fakeJira) seen(substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}

type e2e struct {
	t      *testing.T
	bin    string
	env    []string
	fake   *fakeJira
	cfgDir string
}

func setupE2E(t *testing.T) *e2e {
	t.Helper()
	if testing.Short() {
		t.Skip("e2e builds the binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "jt")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	fake := &fakeJira{}
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	cfg := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(cfg, []byte(fmt.Sprintf(`server: %s
login: aakara
installation: Local
auth_type: basic
project:
  key: DATA
current_project: data
projects:
  data:
    project: {key: DATA}
    board: ""
  ops:
    project: {key: OPS}
    board: ""
`, srv.URL)), 0o600))

	env := append(os.Environ(),
		"JT_CONFIG_FILE="+cfg,
		"JT_API_TOKEN=test-token",
		"XDG_CACHE_HOME="+filepath.Join(dir, "cache"),
		"NO_COLOR=1",
	)
	return &e2e{t: t, bin: bin, env: env, fake: fake, cfgDir: dir}
}

func (e *e2e) run(dir string, args ...string) (string, string, error) {
	cmd := exec.Command(e.bin, args...)
	cmd.Env = e.env
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

func TestE2EFind(t *testing.T) {
	e := setupE2E(t)
	out, stderr, err := e.run("", "find", "retry")
	require.NoError(t, err, stderr)
	assert.Contains(t, out, "DATA-12")
	assert.Contains(t, out, "Add retry to the loader")
	assert.True(t, e.fake.seen(`/rest/api/2/search?jql=project+%3D+%22DATA%22+AND+text+~+%22retry%2A%22`))

	_, _, err = e.run("", "find", "#7", "-p", "ops")
	require.NoError(t, err)
	assert.True(t, e.fake.seen("key+%3D+OPS-7"), "saved project selector sets the key")
}

func TestE2EStandup(t *testing.T) {
	e := setupE2E(t)
	out, stderr, err := e.run("", "standup")
	require.NoError(t, err, stderr)
	assert.Contains(t, out, "**Yesterday**\n- DATA-12 Add retry to the loader — In Progress")
	assert.Contains(t, out, "**Blockers**\n- nothing")
}

func TestE2EAliasAndProjectSwitch(t *testing.T) {
	e := setupE2E(t)
	_, stderr, err := e.run("", "alias", "set", "hello", "!echo hi {me} on {project}")
	require.NoError(t, err, stderr)
	out, _, err := e.run("", "hello")
	require.NoError(t, err)
	assert.Equal(t, "hi aakara on DATA\n", out)

	_, _, err = e.run("", "project", "ops")
	require.NoError(t, err)
	out, _, _ = e.run("", "hello")
	assert.Equal(t, "hi aakara on OPS\n", out)
}

func TestE2EBranchKey(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	e := setupE2E(t)
	repo := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q", "-b", "main")
	git("-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init")

	out, stderr, err := e.run(repo, "branch", "DATA-12", "--print")
	require.NoError(t, err, stderr)
	assert.Equal(t, "feat/DATA-12-add-retry-to-the-loader\n", out)

	_, stderr, err = e.run(repo, "branch", "DATA-12")
	require.NoError(t, err, stderr)
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repo
	head, _ := cmd.Output()
	assert.Equal(t, "feat/DATA-12-add-retry-to-the-loader", strings.TrimSpace(string(head)))

	out, stderr, err = e.run(repo, "issue", "view", "--plain")
	require.NoError(t, err, stderr)
	assert.Contains(t, stderr, "using DATA-12 from the current git branch")
	assert.Contains(t, out, "Add retry to the loader")
}
