package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/aakara-dev/jt/internal/cmd/alias"
	"github.com/aakara-dev/jt/internal/cmd/root"
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
	if exp != nil {
		rootCmd.SetArgs(exp.Args)
	}

	if _, err := rootCmd.ExecuteC(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
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
