package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/term"

	"github.com/aakara-dev/jt/internal/cmd/alias"
	"github.com/aakara-dev/jt/internal/cmd/branchkey"
	"github.com/aakara-dev/jt/internal/cmd/root"
	"github.com/aakara-dev/jt/internal/gitutil"
)

func main() {
	rootCmd := root.NewCmdRoot()

	exp, err := alias.Resolve(rootCmd, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	if exp != nil && exp.Shell != "" {
		os.Exit(runShell(exp.Shell, exp.Args, exp.Env))
	}
	args := os.Args[1:]
	if exp != nil {
		args = exp.Args
	}
	if !wantsHelp(args) {
		var key string
		if args, key = branchkey.Inject(rootCmd, args, keyFromBranch); key != "" {
			note := fmt.Sprintf("using %s from the current git branch", key)
			if term.IsTerminal(int(os.Stderr.Fd())) {
				note = "\x1b[2m" + note + "\x1b[0m"
			}
			fmt.Fprintln(os.Stderr, note)
		}
	}
	rootCmd.SetArgs(args)

	if _, err := rootCmd.ExecuteC(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// keyFromBranch returns the issue key in the current git branch name, if any.
func keyFromBranch() string {
	b, err := gitutil.CurrentBranch()
	if err != nil {
		return ""
	}
	return gitutil.IssueKey(b)
}

// runShell runs a "!" alias with sh, passing args as "$@".
func runShell(script string, args, env []string) int {
	cmd := exec.Command("sh", append([]string{"-c", script, "jt"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	return 0
}
