package backend

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/t0k0sh1/cyclops/internal/gremlins"
	"github.com/t0k0sh1/cyclops/internal/target"
)

type Gremlins struct{}

func (Gremlins) ID() string { return "gremlins" }

func (Gremlins) Capabilities() Capabilities {
	return Capabilities{Diff: true, DryRun: true, ListTargets: true}
}

func (Gremlins) Run(request Request) error {
	if request.List {
		return listGoTargets(request)
	}

	var selection *target.Selection
	if len(request.Targets) > 0 {
		expanded, err := target.Expand(request.Targets)
		if err != nil {
			return usageDiagnostic(err)
		}
		resolved, err := target.Resolve(expanded)
		if err != nil {
			return usageDiagnostic(err)
		}
		selection = &resolved
	}

	args, err := gremlins.Arguments(gremlins.Request{
		Selection: selection,
		DryRun:    request.DryRun,
		DiffBase:  request.DiffBase,
	})
	if err != nil {
		return usageDiagnostic(err)
	}
	err = gremlins.Execute(args, request.Stdin, request.Stdout, request.Stderr)
	if err == nil {
		return nil
	}
	if errors.Is(err, gremlins.ErrNotFound) {
		return &Diagnostic{
			ExitCode: ExitFailure,
			Lines: []string{
				"cyclops: gremlins was not found in PATH",
				"install it with: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest",
			},
			Err: err,
		}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	return &Diagnostic{
		ExitCode: ExitFailure,
		Lines:    []string{fmt.Sprintf("cyclops: failed to run gremlins: %v", err)},
		Err:      err,
	}
}

func listGoTargets(request Request) error {
	var paths []string
	if len(request.Targets) == 0 {
		var err error
		paths, err = target.BelowCurrentDirectory()
		if err != nil {
			return usageDiagnostic(err)
		}
	} else {
		expanded, err := target.Expand(request.Targets)
		if err != nil {
			return usageDiagnostic(err)
		}
		selection, err := target.Resolve(expanded)
		if err != nil {
			return usageDiagnostic(err)
		}
		paths = selection.Paths()
	}

	cwd, err := filepath.Abs(".")
	if err != nil {
		return usageDiagnostic(fmt.Errorf("get current directory: %w", err))
	}
	for _, path := range paths {
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return usageDiagnostic(fmt.Errorf("make target path relative: %w", err))
		}
		fmt.Fprintln(request.Stdout, filepath.ToSlash(rel))
	}
	return nil
}
