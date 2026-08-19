package backend

import (
	"errors"
	"fmt"
	"io"
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
	if request.List && !capabilities.ListTargets {
		return unsupported(selected.ID(), "--list")
	}
	return selected.Run(request)
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
