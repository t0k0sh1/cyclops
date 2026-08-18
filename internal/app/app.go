package app

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/t0k0sh1/cyclops/internal/cli"
	"github.com/t0k0sh1/cyclops/internal/gremlins"
	"github.com/t0k0sh1/cyclops/internal/target"
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
	if options.List {
		if err := printTargets(options.Targets, stdout); err != nil {
			fmt.Fprintf(stderr, "cyclops: %v\n", err)
			return exitUsage
		}
		return 0
	}

	var selection *target.Selection
	if len(options.Targets) > 0 {
		expanded, err := target.Expand(options.Targets)
		if err != nil {
			fmt.Fprintf(stderr, "cyclops: %v\n", err)
			return exitUsage
		}
		resolved, err := target.Resolve(expanded)
		if err != nil {
			fmt.Fprintf(stderr, "cyclops: %v\n", err)
			return exitUsage
		}
		selection = &resolved
	}

	gremlinsArgs, err := gremlins.Arguments(gremlins.Request{
		Selection: selection,
		DryRun:    options.DryRun,
		DiffBase:  options.Diff,
	})
	if err != nil {
		fmt.Fprintf(stderr, "cyclops: %v\n", err)
		return exitUsage
	}
	err = gremlins.Execute(gremlinsArgs, stdin, stdout, stderr)
	if err == nil {
		return 0
	}
	if errors.Is(err, gremlins.ErrNotFound) {
		fmt.Fprintln(stderr, "cyclops: gremlins was not found in PATH")
		fmt.Fprintln(stderr, "install it with: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest")
		return exitFailure
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	fmt.Fprintf(stderr, "cyclops: failed to run gremlins: %v\n", err)
	return exitFailure
}

func printTargets(inputs []string, stdout io.Writer) error {
	var paths []string
	if len(inputs) == 0 {
		var err error
		paths, err = target.BelowCurrentDirectory()
		if err != nil {
			return err
		}
	} else {
		expanded, err := target.Expand(inputs)
		if err != nil {
			return err
		}
		selection, err := target.Resolve(expanded)
		if err != nil {
			return err
		}
		paths = selection.Paths()
	}

	cwd, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
	for _, path := range paths {
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return fmt.Errorf("make target path relative: %w", err)
		}
		fmt.Fprintln(stdout, filepath.ToSlash(rel))
	}
	return nil
}
