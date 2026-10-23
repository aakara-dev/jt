package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/aakara-dev/jt/pkg/jira"
)

// listCacheLimit caps how many issues are stored per query.
const listCacheLimit = 100

// slim copies the fields the list, board and search views render, so cached
// files stay small and never hold descriptions or comments.
func slim(iss *jira.Issue) *jira.Issue {
	out := &jira.Issue{Key: iss.Key}
	f := &out.Fields
	f.Summary = iss.Fields.Summary
	f.Status = iss.Fields.Status
	f.IssueType = iss.Fields.IssueType
	f.Priority = iss.Fields.Priority
	f.Assignee = iss.Fields.Assignee
	f.Reporter = iss.Fields.Reporter
	f.Labels = iss.Fields.Labels
	f.Updated = iss.Fields.Updated
	f.Created = iss.Fields.Created
	f.Watches = iss.Fields.Watches
	return out
}

// listCachePath is ~/.cache/jt/lists/<sha256(server|jql)>.json.
func listCachePath(server, jql string) string {
	dir := cacheDir()
	if dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(server + "|" + jql))
	return filepath.Join(dir, "lists", hex.EncodeToString(sum[:12])+".json")
}

// loadListCache returns the last results seen for jql, if any. The cache is
// a stale-while-revalidate convenience: views always refetch.
func loadListCache(server, jql string) []*jira.Issue {
	path := listCachePath(server, jql)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var issues []*jira.Issue
	if json.Unmarshal(data, &issues) != nil {
		return nil
	}
	return issues
}

// saveListCache stores the first page of results for jql (best effort).
func saveListCache(server, jql string, issues []*jira.Issue) {
	path := listCachePath(server, jql)
	if path == "" {
		return
	}
	n := min(len(issues), listCacheLimit)
	out := make([]*jira.Issue, 0, n)
	for _, iss := range issues[:n] {
		out = append(out, slim(iss))
	}
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
