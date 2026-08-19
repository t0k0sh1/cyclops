package backend

import (
	"errors"
	"fmt"
	"io"

	mutationresult "github.com/t0k0sh1/cyclops/internal/result"
)

const (
	ExitFailure = 1
	ExitUsage   = 2
)

type Capabilities struct {
	Diff        bool
	DryRun      bool
	ListTargets bool
}

type Request struct {
	Targets  []string
	DryRun   bool
	DiffBase string
	List     bool
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

type Backend interface {
	ID() string
	Capabilities() Capabilities
	Run(Request) (mutationresult.Run, error)
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

func Execute(selected Backend, request Request) (mutationresult.Run, error) {
	capabilities := selected.Capabilities()
	if request.DiffBase != "" && !capabilities.Diff {
		return mutationresult.Run{}, unsupported(selected.ID(), "--diff")
	}
	if request.DryRun && !capabilities.DryRun {
		return mutationresult.Run{}, unsupported(selected.ID(), "--dry-run")
	}
	if request.List && !capabilities.ListTargets {
		return mutationresult.Run{}, unsupported(selected.ID(), "--list")
	}
	return selected.Run(request)
}

func normalizedRun(id string, request Request) mutationresult.Run {
	return mutationresult.Run{
		Backend: mutationresult.Backend{ID: id},
		Scope: mutationresult.Scope{
			DiffBase: request.DiffBase,
			Targets:  append([]string(nil), request.Targets...),
		},
		State:  mutationresult.StateComplete,
		DryRun: request.DryRun,
	}
}

func recordExecutionError(run *mutationresult.Run, kind, message string, exitCode *int) {
	run.Errors = append(run.Errors, mutationresult.ExecutionError{
		Kind:     kind,
		Message:  message,
		ExitCode: exitCode,
	})
	if len(run.Mutants) == 0 {
		run.State = mutationresult.StateFailed
	} else {
		run.State = mutationresult.StatePartial
	}
}

func unsupported(id, option string) error {
	message := fmt.Sprintf("cyclops: backend %q does not support %s", id, option)
	return &Diagnostic{ExitCode: ExitUsage, Lines: []string{message}, Err: errors.New(message)}
}

func Default() Backend { return Gremlins{} }

func usageDiagnostic(err error) error {
	return &Diagnostic{
		ExitCode: ExitUsage,
		Lines:    []string{fmt.Sprintf("cyclops: %v", err)},
		Err:      err,
	}
}

type ProcessExit struct {
	BackendID string
	Code      int
	Meaning   string
	Err       error
}

func (e *ProcessExit) Error() string { return e.Err.Error() }
func (e *ProcessExit) Unwrap() error { return e.Err }
