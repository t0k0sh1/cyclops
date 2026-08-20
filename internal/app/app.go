package app

import (
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/t0k0sh1/cyclops/internal/affected"
	"github.com/t0k0sh1/cyclops/internal/backend"
	"github.com/t0k0sh1/cyclops/internal/cli"
)

const (
	exitFailure = 1
	exitUsage   = 2
)

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, err := cli.Parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "cyclops: %v\n", err)
		return exitUsage
	}
	if options.Help {
		cli.PrintHelp(stdout)
		return 0
	}
	if options.Version {
		fmt.Fprintf(stdout, "cyclops %s\n", cli.Version)
		return 0
	}
	if options.ListTestTargets != "" {
		targets, err := affected.TestTargets(".", options.ListTestTargets)
		if err != nil {
			fmt.Fprintf(stderr, "cyclops: list test-affected targets: %v\n", err)
			return exitFailure
		}
		for _, target := range targets {
			fmt.Fprintln(stdout, target)
		}
		return 0
	}
	selected, err := backend.Select(".")
	if err != nil {
		var diagnostic *backend.Diagnostic
		if errors.As(err, &diagnostic) {
			for _, line := range diagnostic.Lines {
				fmt.Fprintln(stderr, line)
			}
			return diagnostic.ExitCode
		}
		fmt.Fprintf(stderr, "cyclops: select backend: %v\n", err)
		return exitFailure
	}

	_, err = backend.Execute(selected, backend.Request{
		Targets:  options.Targets,
		DryRun:   options.DryRun,
		DiffBase: options.Diff,
		List:     options.List,
		Stdin:    stdin,
		Stdout:   stdout,
		Stderr:   stderr,
	})
	if err == nil {
		return 0
	}
	var diagnostic *backend.Diagnostic
	if errors.As(err, &diagnostic) {
		for _, line := range diagnostic.Lines {
			fmt.Fprintln(stderr, line)
		}
		return diagnostic.ExitCode
	}
	var processExit *backend.ProcessExit
	if errors.As(err, &processExit) {
		return processExit.Code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	fmt.Fprintf(stderr, "cyclops: backend %q failed: %v\n", selected.ID(), err)
	return exitFailure
}
