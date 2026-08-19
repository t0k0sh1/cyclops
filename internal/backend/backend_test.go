package backend

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestSelectsBackendFromProjectMarker(t *testing.T) {
	tests := []struct {
		name   string
		marker string
		want   string
	}{
		{name: "Go", marker: "go.mod", want: "gremlins"},
		{name: "Rust", marker: "Cargo.toml", want: "cargo-mutants"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, test.marker), []byte(""), 0o644); err != nil {
				t.Fatal(err)
			}
			nested := filepath.Join(root, "src", "nested")
			if err := os.MkdirAll(nested, 0o755); err != nil {
				t.Fatal(err)
			}
			selected, err := Select(nested)
			if err != nil {
				t.Fatal(err)
			}
			if got := selected.ID(); got != test.want {
				t.Errorf("selected backend = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSelectRejectsAmbiguousProject(t *testing.T) {
	root := t.TempDir()
	for _, marker := range []string{"go.mod", "Cargo.toml"} {
		if err := os.WriteFile(filepath.Join(root, marker), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Select(root)
	var diagnostic *Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("Select() error = %v, want Diagnostic", err)
	}
	if !strings.Contains(diagnostic.Error(), "ambiguous mutation backend") {
		t.Errorf("Select() error = %q", diagnostic.Error())
	}
}

func TestCargoMutantsArguments(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src", "lib.rs")
	if err := os.Mkdir(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("pub fn value() -> i32 { 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selected := CargoMutants{Root: root}
	args, err := selected.arguments(Request{Targets: []string{source}, DryRun: true}, "/tmp/change.diff")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mutants", "--file", "src/lib.rs", "--in-diff", "/tmp/change.diff", "--list", "--json"}
	if got := strings.Join(args, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("arguments = %q, want %q", args, want)
	}
}

func TestCargoMutantsListArguments(t *testing.T) {
	selected := CargoMutants{Root: t.TempDir()}
	args, err := selected.arguments(Request{List: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(args, " "), "mutants --list-files"; got != want {
		t.Errorf("arguments = %q, want %q", got, want)
	}
}

func TestCargoMutantsMissingExecutableDiagnostic(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := (CargoMutants{Root: t.TempDir()}).Run(Request{
		Stdin:  strings.NewReader(""),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	var diagnostic *Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("Run() error = %v, want Diagnostic", err)
	}
	if got := strings.Join(diagnostic.Lines, "\n"); !strings.Contains(got, "cargo install --locked cargo-mutants") {
		t.Errorf("diagnostic = %q", got)
	}
}

func TestCargoMutantsPreservesExitMeaning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "cargo-mutants")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	err := (CargoMutants{Root: t.TempDir()}).Run(Request{
		Stdin:  strings.NewReader(""),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	var processExit *ProcessExit
	if !errors.As(err, &processExit) {
		t.Fatalf("Run() error = %v, want ProcessExit", err)
	}
	if processExit.Code != 2 || processExit.Meaning != "surviving mutants" {
		t.Errorf("process exit = %+v", processExit)
	}
}

func TestCargoMutantsWritesWorkingTreeDiff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	runGit("init")
	source := filepath.Join(root, "src", "lib.rs")
	if err := os.Mkdir(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("pub fn value() -> i32 { 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "src/lib.rs")
	runGit("-c", "user.name=Cyclops Test", "-c", "user.email=cyclops@example.invalid", "commit", "-m", "initial")
	if err := os.WriteFile(source, []byte("pub fn value() -> i32 { 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffPath, err := (CargoMutants{Root: root}).writeDiff("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(diffPath)
	diff, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(diff), "+pub fn value() -> i32 { 2 }") {
		t.Errorf("diff does not contain working tree version:\n%s", diff)
	}
}
