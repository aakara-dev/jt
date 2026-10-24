// Package gitutil wraps the few git operations jt needs.
package gitutil

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ErrNotARepo is returned outside a git work tree.
var ErrNotARepo = errors.New("not inside a git repository")

var keyRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9_]+-\d+)\b`)

// IssueKey extracts the first issue key from a branch name, e.g.
// "feat/DATA-12-add-retry" → "DATA-12". It returns "" if there is none.
func IssueKey(branch string) string {
	m := keyRe.FindStringSubmatch(branch)
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1])
}

func run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if strings.Contains(msg, "not a git repository") {
			return "", ErrNotARepo
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// CurrentBranch returns the checked-out branch name.
func CurrentBranch() (string, error) {
	return run("rev-parse", "--abbrev-ref", "HEAD")
}

// BranchExists reports whether a local branch exists.
func BranchExists(name string) bool {
	_, err := run("rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// Switch checks out name, creating it from base (or HEAD) if it is new.
// It reports whether the branch was created.
func Switch(name, base string) (bool, error) {
	if BranchExists(name) {
		_, err := run("switch", name)
		return false, err
	}
	args := []string{"switch", "-c", name}
	if base != "" {
		args = append(args, base)
	}
	_, err := run(args...)
	return err == nil, err
}
