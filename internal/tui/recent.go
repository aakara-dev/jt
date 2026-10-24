package tui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/aakara-dev/jt/pkg/jira"
)

const recentLimit = 50

// recentStore keeps recently viewed issues on disk so the search window can
// show them instantly before any query runs.
type recentStore struct {
	path   string
	issues []*jira.Issue
}

func cacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "jt")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "jt")
}

func loadRecent() *recentStore {
	r := &recentStore{}
	dir := cacheDir()
	if dir == "" {
		return r
	}
	r.path = filepath.Join(dir, "recent.json")
	if data, err := os.ReadFile(r.path); err == nil {
		_ = json.Unmarshal(data, &r.issues)
	}
	return r
}

// add moves iss to the front, keeping a trimmed copy.
func (r *recentStore) add(iss *jira.Issue) {
	if iss == nil {
		return
	}
	out := []*jira.Issue{slim(iss)}
	for _, x := range r.issues {
		if x.Key != iss.Key && len(out) < recentLimit {
			out = append(out, x)
		}
	}
	r.issues = out
	r.save()
}

// save is best effort: the cache is a convenience, never a source of truth.
func (r *recentStore) save() {
	if r.path == "" {
		return
	}
	data, err := json.Marshal(r.issues)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(r.path, data, 0o600)
}
