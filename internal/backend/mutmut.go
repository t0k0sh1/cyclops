package backend

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	mutationresult "github.com/t0k0sh1/cyclops/internal/result"
)

type Mutmut struct {
	Root string
}

func (Mutmut) ID() string { return "mutmut" }

func (Mutmut) Capabilities() Capabilities {
	return Capabilities{Diff: true}
}

func (backend Mutmut) Run(request Request) (mutationresult.Run, error) {
	run := normalizedRun(backend.ID(), request)
	patterns, err := backend.mutantPatterns(request.Targets, request.DiffBase)
	if err != nil {
		return run, usageDiagnostic(err)
	}
	if request.DiffBase != "" && len(patterns) == 0 {
		run.State = mutationresult.StateNoCandidates
		return run, nil
	}
	executable, err := backend.executable()
	if err != nil {
		recordExecutionError(&run, "tool", err.Error(), nil)
		return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{
			"cyclops: mutmut was not found in .venv/bin, venv/bin, or PATH",
			"install it with: python -m pip install mutmut",
		}, Err: err}
	}
	args := append([]string{"run"}, patterns...)
	cmd := exec.Command(executable, args...)
	cmd.Dir = backend.Root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = request.Stdin, request.Stdout, request.Stderr
	executionErr := cmd.Run()
	if executionErr == nil {
		export := exec.Command(executable, "export-cicd-stats")
		export.Dir = backend.Root
		export.Stdin, export.Stdout, export.Stderr = request.Stdin, request.Stdout, request.Stderr
		executionErr = export.Run()
	}
	parsed, parseErr := parseMutmutStatsFile(filepath.Join(backend.Root, "mutants", "mutmut-cicd-stats.json"), request)
	if parseErr == nil {
		run = parsed
	} else if executionErr == nil {
		run.State = mutationresult.StatePartial
		run.Errors = append(run.Errors, mutationresult.ExecutionError{Kind: "parse", Message: parseErr.Error()})
	}
	if executionErr == nil {
		return run, nil
	}
	var exitErr *exec.ExitError
	if errors.As(executionErr, &exitErr) {
		code := exitErr.ExitCode()
		if parseErr != nil {
			recordExecutionError(&run, "tool", executionErr.Error(), &code)
		}
		return run, &ProcessExit{BackendID: backend.ID(), Code: code, Meaning: "mutmut or its baseline tests failed", Err: executionErr}
	}
	recordExecutionError(&run, "tool", executionErr.Error(), nil)
	return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: failed to run mutmut: %v", executionErr)}, Err: executionErr}
}

func (backend Mutmut) executable() (string, error) {
	for _, relative := range []string{filepath.Join(".venv", "bin", "mutmut"), filepath.Join("venv", "bin", "mutmut")} {
		path := filepath.Join(backend.Root, relative)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return exec.LookPath("mutmut")
}

func parseMutmutStatsFile(path string, request Request) (mutationresult.Run, error) {
	file, err := os.Open(path)
	if err != nil {
		return mutationresult.Run{}, err
	}
	defer file.Close()
	return mutationresult.ParseMutmutStats(file, mutationresult.Scope{DiffBase: request.DiffBase, Targets: append([]string(nil), request.Targets...)})
}

func (backend Mutmut) mutantPatterns(inputs []string, diffBase string) ([]string, error) {
	paths := inputs
	if diffBase != "" {
		changed, err := backend.changedPythonFiles(diffBase)
		if err != nil {
			return nil, err
		}
		paths = changed
		if len(inputs) > 0 {
			allowed := map[string]struct{}{}
			for _, input := range inputs {
				pattern, patternErr := backend.modulePattern(input)
				if patternErr != nil {
					return nil, patternErr
				}
				allowed[pattern] = struct{}{}
			}
			paths = paths[:0]
			for _, changedPath := range changed {
				pattern, _ := backend.modulePattern(filepath.Join(backend.Root, changedPath))
				if _, ok := allowed[pattern]; ok {
					paths = append(paths, changedPath)
				}
			}
		}
	}
	unique := map[string]struct{}{}
	for _, path := range paths {
		if diffBase != "" {
			path = filepath.Join(backend.Root, path)
		}
		pattern, err := backend.modulePattern(path)
		if err != nil {
			return nil, err
		}
		unique[pattern] = struct{}{}
	}
	patterns := make([]string, 0, len(unique))
	for pattern := range unique {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	return patterns, nil
}

func (backend Mutmut) changedPythonFiles(base string) ([]string, error) {
	if strings.HasPrefix(base, "-") {
		return nil, fmt.Errorf("invalid diff reference: %s", base)
	}
	cmd := exec.Command("git", "diff", "--name-only", "-z", "--diff-filter=ACMRTUXB", base, "--")
	cmd.Dir = backend.Root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("generate diff from %s: %s", base, detail)
	}
	var files []string
	for _, raw := range bytes.Split(output, []byte{0}) {
		path := string(raw)
		if strings.HasSuffix(path, ".py") && !strings.HasPrefix(filepath.Base(path), "test_") && !strings.HasSuffix(path, "_test.py") {
			files = append(files, path)
		}
	}
	return files, nil
}

func (backend Mutmut) modulePattern(input string) (string, error) {
	if strings.ContainsAny(input, "*?[") {
		return "", fmt.Errorf("mutmut targets must be explicit Python files; patterns are not supported: %s", input)
	}
	absolute, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolve target %s: %w", input, err)
	}
	relative, err := filepath.Rel(backend.Root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("target is outside mutmut project %s: %s", backend.Root, input)
	}
	path := filepath.ToSlash(relative)
	if !strings.HasSuffix(path, ".py") {
		return "", fmt.Errorf("mutmut target must be a Python source file: %s", input)
	}
	if strings.HasPrefix(filepath.Base(path), "test_") || strings.HasSuffix(path, "_test.py") {
		return "", fmt.Errorf("test files cannot be mutation targets: %s", input)
	}
	if info, statErr := os.Stat(absolute); statErr != nil {
		return "", fmt.Errorf("cannot access target %s: %w", input, statErr)
	} else if !info.Mode().IsRegular() {
		return "", fmt.Errorf("target is not a regular file: %s", input)
	}
	path = strings.TrimSuffix(path, ".py")
	path = strings.TrimPrefix(path, "src/")
	path = strings.TrimSuffix(path, "/__init__")
	return strings.ReplaceAll(path, "/", ".") + "*", nil
}
