package backend

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CargoMutants struct {
	Root string
}

func (CargoMutants) ID() string { return "cargo-mutants" }

func (CargoMutants) Capabilities() Capabilities {
	return Capabilities{Diff: true, DryRun: true, ListTargets: true}
}

func (backend CargoMutants) Run(request Request) error {
	path, err := exec.LookPath("cargo-mutants")
	if err != nil {
		return &Diagnostic{
			ExitCode: ExitFailure,
			Lines: []string{
				"cyclops: cargo-mutants was not found in PATH",
				"install it with: cargo install --locked cargo-mutants",
			},
			Err: err,
		}
	}

	var diffPath string
	if request.DiffBase != "" {
		diffPath, err = backend.writeDiff(request.DiffBase)
		if err != nil {
			return usageDiagnostic(err)
		}
		defer os.Remove(diffPath)
	}
	args, err := backend.arguments(request, diffPath)
	if err != nil {
		return usageDiagnostic(err)
	}

	cmd := exec.Command(path, args...)
	cmd.Dir = backend.Root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = request.Stdin, request.Stdout, request.Stderr
	err = cmd.Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ProcessExit{
			BackendID: backend.ID(),
			Code:      exitErr.ExitCode(),
			Meaning:   cargoMutantsExitMeaning(exitErr.ExitCode()),
			Err:       err,
		}
	}
	return &Diagnostic{
		ExitCode: ExitFailure,
		Lines:    []string{fmt.Sprintf("cyclops: failed to run cargo-mutants: %v", err)},
		Err:      err,
	}
}

func (backend CargoMutants) arguments(request Request, diffPath string) ([]string, error) {
	args := []string{"mutants"}
	for _, input := range request.Targets {
		target, err := backend.targetPattern(input)
		if err != nil {
			return nil, err
		}
		args = append(args, "--file", target)
	}
	if diffPath != "" {
		args = append(args, "--in-diff", diffPath)
	}
	if request.List {
		args = append(args, "--list-files")
	} else if request.DryRun {
		args = append(args, "--list", "--json")
	}
	return args, nil
}

func (backend CargoMutants) targetPattern(input string) (string, error) {
	if !strings.HasSuffix(input, ".rs") {
		return "", fmt.Errorf("target must be a .rs file or pattern: %s", input)
	}
	absolute, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolve target %s: %w", input, err)
	}
	relative, err := filepath.Rel(backend.Root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("target is outside Cargo project %s: %s", backend.Root, input)
	}
	if !strings.ContainsAny(input, "*?[{") {
		info, err := os.Stat(absolute)
		if err != nil {
			return "", fmt.Errorf("cannot access target %s: %w", input, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("target is not a regular file: %s", input)
		}
	}
	return filepath.ToSlash(relative), nil
}

func (backend CargoMutants) writeDiff(base string) (string, error) {
	if strings.HasPrefix(base, "-") {
		return "", fmt.Errorf("invalid diff reference: %s", base)
	}
	cmd := exec.Command("git", "diff", "--no-ext-diff", "--unified=0", base, "--")
	cmd.Dir = backend.Root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	contents, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("generate diff from %s: %s", base, detail)
	}
	file, err := os.CreateTemp("", "cyclops-cargo-mutants-*.diff")
	if err != nil {
		return "", fmt.Errorf("create cargo-mutants diff: %w", err)
	}
	name := file.Name()
	if _, err := file.Write(contents); err != nil {
		file.Close()
		os.Remove(name)
		return "", fmt.Errorf("write cargo-mutants diff: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("close cargo-mutants diff: %w", err)
	}
	return name, nil
}

func cargoMutantsExitMeaning(code int) string {
	switch code {
	case 1:
		return "usage error"
	case 2:
		return "surviving mutants"
	case 3:
		return "timed-out mutants"
	case 4:
		return "baseline tests failed or timed out"
	case 5:
		return "diff does not match the working tree"
	case 6:
		return "invalid diff"
	case 70:
		return "internal cargo-mutants error"
	default:
		return "cargo-mutants failed"
	}
}
