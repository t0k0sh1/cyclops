package backend

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/t0k0sh1/cyclops/internal/gremlins"
	mutationresult "github.com/t0k0sh1/cyclops/internal/result"
	"github.com/t0k0sh1/cyclops/internal/target"
)

type Gremlins struct{}

func (Gremlins) ID() string { return "gremlins" }

func (Gremlins) Capabilities() Capabilities {
	return Capabilities{Diff: true, DryRun: true, ListTargets: true}
}

func (Gremlins) Run(request Request) (mutationresult.Run, error) {
	if request.List {
		err := listGoTargets(request)
		run := normalizedRun("gremlins", request)
		run.State = mutationresult.StateSkipped
		return run, err
	}

	var selection *target.Selection
	if len(request.Targets) > 0 {
		expanded, err := target.Expand(request.Targets)
		if err != nil {
			return mutationresult.Run{}, usageDiagnostic(err)
		}
		resolved, err := target.Resolve(expanded)
		if err != nil {
			return mutationresult.Run{}, usageDiagnostic(err)
		}
		selection = &resolved
	}

	output, err := os.CreateTemp("", "cyclops-gremlins-*.json")
	if err != nil {
		return mutationresult.Run{}, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: create Gremlins result file: %v", err)}, Err: err}
	}
	outputPath := output.Name()
	if err := output.Close(); err != nil {
		os.Remove(outputPath)
		return mutationresult.Run{}, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: close Gremlins result file: %v", err)}, Err: err}
	}
	defer os.Remove(outputPath)

	args, err := gremlins.Arguments(gremlins.Request{
		Selection: selection,
		DryRun:    request.DryRun,
		DiffBase:  request.DiffBase,
		Output:    outputPath,
	})
	if err != nil {
		return mutationresult.Run{}, usageDiagnostic(err)
	}
	executionErr := gremlins.Execute(args, request.Stdin, request.Stdout, request.Stderr)
	run := normalizedRun("gremlins", request)
	if parsed, parseErr := parseGremlinsFile(outputPath, request); parseErr == nil {
		run = parsed
	} else if executionErr == nil {
		run.State = mutationresult.StatePartial
		run.Errors = append(run.Errors, mutationresult.ExecutionError{Kind: "parse", Message: parseErr.Error()})
	}
	if executionErr == nil {
		return run, nil
	}
	if errors.Is(executionErr, gremlins.ErrNotFound) {
		recordExecutionError(&run, "tool", executionErr.Error(), nil)
		return run, &Diagnostic{
			ExitCode: ExitFailure,
			Lines: []string{
				"cyclops: gremlins was not found in PATH",
				"install it with: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest",
			},
			Err: executionErr,
		}
	}
	var exitErr *exec.ExitError
	if errors.As(executionErr, &exitErr) {
		code := exitErr.ExitCode()
		recordExecutionError(&run, "process", executionErr.Error(), &code)
		return run, executionErr
	}
	recordExecutionError(&run, "tool", executionErr.Error(), nil)
	return run, &Diagnostic{
		ExitCode: ExitFailure,
		Lines:    []string{fmt.Sprintf("cyclops: failed to run gremlins: %v", executionErr)},
		Err:      executionErr,
	}
}

func parseGremlinsFile(path string, request Request) (mutationresult.Run, error) {
	file, err := os.Open(path)
	if err != nil {
		return mutationresult.Run{}, err
	}
	defer file.Close()
	return mutationresult.ParseGremlins(file, mutationresult.Scope{
		DiffBase: request.DiffBase,
		Targets:  append([]string(nil), request.Targets...),
	}, request.DryRun)
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
