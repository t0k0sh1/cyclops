package backend

import (
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/t0k0sh1/cyclops/internal/gremlins"
	"github.com/t0k0sh1/cyclops/internal/target"
)

const (
	ExitFailure = 1
	ExitUsage   = 2
)

type Capabilities struct {
	Diff   bool
	DryRun bool
}

type Request struct {
	Targets  []string
	DryRun   bool
	DiffBase string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

type Backend interface {
	ID() string
	Capabilities() Capabilities
	Run(Request) error
}

type Diagnostic struct {
	ExitCode int
	Lines    []string
	Err      error
}

func (d *Diagnostic) Error() string {
	if d.Err != nil {
		return d.Err.Error()
	}
	if len(d.Lines) > 0 {
		return d.Lines[0]
	}
	return "backend failed"
}

func (d *Diagnostic) Unwrap() error { return d.Err }

func Execute(selected Backend, request Request) error {
	capabilities := selected.Capabilities()
	if request.DiffBase != "" && !capabilities.Diff {
		return unsupported(selected.ID(), "--diff")
	}
	if request.DryRun && !capabilities.DryRun {
		return unsupported(selected.ID(), "--dry-run")
	}
	return selected.Run(request)
}

func unsupported(id, option string) error {
	message := fmt.Sprintf("cyclops: backend %q does not support %s", id, option)
	return &Diagnostic{ExitCode: ExitUsage, Lines: []string{message}, Err: errors.New(message)}
}

func Default() Backend { return Gremlins{} }

type Gremlins struct{}

func (Gremlins) ID() string { return "gremlins" }

func (Gremlins) Capabilities() Capabilities {
	return Capabilities{Diff: true, DryRun: true}
}

func (Gremlins) Run(request Request) error {
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

func usageDiagnostic(err error) error {
	return &Diagnostic{
		ExitCode: ExitUsage,
		Lines:    []string{fmt.Sprintf("cyclops: %v", err)},
		Err:      err,
	}
}
