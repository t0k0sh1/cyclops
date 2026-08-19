package backend

import (
	"errors"
	"testing"
)

type stubBackend struct {
	id           string
	capabilities Capabilities
	called       bool
}

func (b *stubBackend) ID() string                 { return b.id }
func (b *stubBackend) Capabilities() Capabilities { return b.capabilities }
func (b *stubBackend) Run(Request) error          { b.called = true; return nil }

func TestExecuteRejectsUnsupportedDiff(t *testing.T) {
	selected := &stubBackend{id: "example", capabilities: Capabilities{DryRun: true}}
	err := Execute(selected, Request{DiffBase: "origin/main"})

	var diagnostic *Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("Execute() error = %v, want Diagnostic", err)
	}
	if diagnostic.ExitCode != ExitUsage {
		t.Errorf("diagnostic exit code = %d, want %d", diagnostic.ExitCode, ExitUsage)
	}
	if got, want := diagnostic.Lines[0], `cyclops: backend "example" does not support --diff`; got != want {
		t.Errorf("diagnostic = %q, want %q", got, want)
	}
	if selected.called {
		t.Error("backend was called for an unsupported request")
	}
}

func TestExecuteRejectsUnsupportedDryRun(t *testing.T) {
	selected := &stubBackend{id: "example", capabilities: Capabilities{Diff: true}}
	err := Execute(selected, Request{DryRun: true})

	var diagnostic *Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("Execute() error = %v, want Diagnostic", err)
	}
	if got, want := diagnostic.Lines[0], `cyclops: backend "example" does not support --dry-run`; got != want {
		t.Errorf("diagnostic = %q, want %q", got, want)
	}
	if selected.called {
		t.Error("backend was called for an unsupported request")
	}
}

func TestExecuteRunsSupportedRequest(t *testing.T) {
	selected := &stubBackend{
		id:           "example",
		capabilities: Capabilities{Diff: true, DryRun: true},
	}
	if err := Execute(selected, Request{DiffBase: "HEAD~1", DryRun: true}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !selected.called {
		t.Error("backend was not called")
	}
}

func TestDefaultIsGremlins(t *testing.T) {
	selected := Default()
	if got, want := selected.ID(), "gremlins"; got != want {
		t.Errorf("default backend ID = %q, want %q", got, want)
	}
	if got := selected.Capabilities(); !got.Diff || !got.DryRun {
		t.Errorf("default capabilities = %+v, want diff and dry-run", got)
	}
}
