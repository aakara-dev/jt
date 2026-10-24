// Package branchkey fills in a missing ISSUE-KEY argument from the current
// git branch name, so `jt issue view` on feat/DATA-12-x views DATA-12.
package branchkey

import (
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// commands take ISSUE-KEY as their first positional argument. Destructive
// commands (delete) are deliberately not listed.
var commands = map[string]bool{
	"jt issue view":        true,
	"jt issue move":        true,
	"jt issue assign":      true,
	"jt issue edit":        true,
	"jt issue watch":       true,
	"jt issue clone":       true,
	"jt issue comment add": true,
	"jt issue worklog add": true,
	"jt branch":            true,
}

var keyLike = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]+-)?\d+$`)

// Inject returns args with the branch's issue key inserted where the
// command expects one, plus the key used ("" if args are unchanged).
// branchKey is only called when a key is actually needed.
func Inject(root *cobra.Command, args []string, branchKey func() string) ([]string, string) {
	if len(args) == 1 && args[0] == "." {
		if key := branchKey(); key != "" {
			return []string{"ui", key}, key
		}
		return args, ""
	}
	cmd, rest, err := root.Find(args)
	if err != nil || cmd == root || !commands[cmd.CommandPath()] {
		return args, ""
	}
	pos, idx := firstPositional(cmd, rest)
	if pos != "" && keyLike.MatchString(pos) {
		return args, ""
	}
	key := branchKey()
	if key == "" {
		return args, ""
	}
	out := strings.Fields(cmd.CommandPath())[1:]
	out = append(out, rest[:idx]...)
	out = append(out, key)
	out = append(out, rest[idx:]...)
	return out, key
}

// firstPositional finds the first non-flag argument, skipping flag values.
// It returns the argument and its index, or ("", len(args)).
func firstPositional(cmd *cobra.Command, args []string) (string, int) {
	flags := cmd.Flags()
	lookup := func(name string, short bool) *pflag.Flag {
		if short {
			if f := flags.ShorthandLookup(name); f != nil {
				return f
			}
			return cmd.InheritedFlags().ShorthandLookup(name)
		}
		if f := flags.Lookup(name); f != nil {
			return f
		}
		return cmd.InheritedFlags().Lookup(name)
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			if i+1 < len(args) {
				return args[i+1], i + 1
			}
			return "", len(args)
		case strings.HasPrefix(a, "--"):
			name, _, hasValue := strings.Cut(a[2:], "=")
			if f := lookup(name, false); f != nil && f.NoOptDefVal == "" && !hasValue {
				i++
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// -x, -xVALUE, or -x VALUE
			if f := lookup(a[1:2], true); f != nil && f.NoOptDefVal == "" && len(a) == 2 {
				i++
			}
		default:
			return a, i
		}
	}
	return "", len(args)
}
