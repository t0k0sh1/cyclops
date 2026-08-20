package backend

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	mutationresult "github.com/t0k0sh1/cyclops/internal/result"
)

type PIT struct {
	Root string
}

func (PIT) ID() string { return "pit" }

func (PIT) Capabilities() Capabilities {
	return Capabilities{Diff: true, DryRun: true}
}

func (backend PIT) Run(request Request) (mutationresult.Run, error) {
	run := normalizedRun(backend.ID(), request)
	classes, err := backend.targetClasses(request.Targets, request.DiffBase)
	if err != nil {
		return run, usageDiagnostic(err)
	}
	if request.DiffBase != "" && len(classes) == 0 {
		run.State = mutationresult.StateNoCandidates
		return run, nil
	}
	executable, err := backend.executable()
	if err != nil {
		recordExecutionError(&run, "tool", err.Error(), nil)
		return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{
			"cyclops: Maven was not found as ./mvnw or in PATH",
			"install Maven or add the Maven wrapper to the project",
		}, Err: err}
	}
	reportDir, err := os.MkdirTemp("", "cyclops-pit-*")
	if err != nil {
		return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: create PIT result directory: %v", err)}, Err: err}
	}
	defer os.RemoveAll(reportDir)
	args := []string{
		"-DoutputFormats=XML", "-DtimestampedReports=false", "-DreportsDirectory=" + reportDir,
	}
	if len(classes) > 0 {
		args = append(args, "-DtargetClasses="+strings.Join(classes, ","))
	}
	if request.DryRun {
		args = append(args, "-Dpit.dryRun=true")
	}
	args = append(args, "test-compile", "org.pitest:pitest-maven:mutationCoverage")
	cmd := exec.Command(executable, args...)
	cmd.Dir = backend.Root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = request.Stdin, request.Stdout, request.Stderr
	executionErr := cmd.Run()
	parsed, parseErr := parsePITFile(filepath.Join(reportDir, "mutations.xml"), request)
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
		return run, &ProcessExit{BackendID: backend.ID(), Code: code, Meaning: "Maven or PIT failed", Err: executionErr}
	}
	recordExecutionError(&run, "tool", executionErr.Error(), nil)
	return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: failed to run PIT: %v", executionErr)}, Err: executionErr}
}

func (backend PIT) executable() (string, error) {
	for _, name := range []string{"mvnw", "mvnw.cmd"} {
		path := filepath.Join(backend.Root, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return exec.LookPath("mvn")
}

func parsePITFile(path string, request Request) (mutationresult.Run, error) {
	file, err := os.Open(path)
	if err != nil {
		return mutationresult.Run{}, err
	}
	defer file.Close()
	return mutationresult.ParsePIT(file, mutationresult.Scope{DiffBase: request.DiffBase, Targets: append([]string(nil), request.Targets...)}, request.DryRun)
}

func (backend PIT) targetClasses(inputs []string, diffBase string) ([]string, error) {
	var paths []string
	if diffBase != "" {
		changed, err := backend.changedJavaFiles(diffBase)
		if err != nil {
			return nil, err
		}
		paths = changed
		if len(inputs) > 0 {
			allowed := make([]string, 0, len(inputs))
			for _, input := range inputs {
				class, classErr := backend.javaClass(input)
				if classErr != nil {
					return nil, classErr
				}
				allowed = append(allowed, class)
			}
			paths = paths[:0]
			for _, changedPath := range changed {
				class, _ := backend.javaClass(filepath.Join(backend.Root, changedPath))
				name := strings.TrimSuffix(class, "*")
				for _, pattern := range allowed {
					matched, matchErr := path.Match(pattern, name)
					if matchErr != nil {
						return nil, fmt.Errorf("invalid PIT target pattern %q: %w", pattern, matchErr)
					}
					if matched {
						paths = append(paths, changedPath)
						break
					}
				}
			}
		}
	} else {
		paths = inputs
	}
	unique := map[string]struct{}{}
	for _, path := range paths {
		if diffBase != "" {
			path = filepath.Join(backend.Root, path)
		}
		class, err := backend.javaClass(path)
		if err != nil {
			return nil, err
		}
		unique[class] = struct{}{}
	}
	classes := make([]string, 0, len(unique))
	for class := range unique {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	return classes, nil
}

func (backend PIT) changedJavaFiles(base string) ([]string, error) {
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
		slashed := filepath.ToSlash(path)
		if strings.HasSuffix(path, ".java") && (strings.Contains(slashed, "/src/main/java/") || strings.HasPrefix(slashed, "src/main/java/")) {
			files = append(files, path)
		}
	}
	return files, nil
}

func (backend PIT) javaClass(input string) (string, error) {
	absolute, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolve target %s: %w", input, err)
	}
	relative, err := filepath.Rel(backend.Root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("target is outside PIT project %s: %s", backend.Root, input)
	}
	path := filepath.ToSlash(relative)
	marker := "src/main/java/"
	index := strings.Index(path, marker)
	if index < 0 || !strings.HasSuffix(path, ".java") {
		return "", fmt.Errorf("PIT target must be a Java source below src/main/java: %s", input)
	}
	if !strings.ContainsAny(input, "*?[") {
		if info, statErr := os.Stat(absolute); statErr != nil {
			return "", fmt.Errorf("cannot access target %s: %w", input, statErr)
		} else if !info.Mode().IsRegular() {
			return "", fmt.Errorf("target is not a regular file: %s", input)
		}
	}
	class := strings.TrimSuffix(path[index+len(marker):], ".java")
	class = strings.ReplaceAll(class, "/", ".")
	class = strings.ReplaceAll(class, "**", "*")
	if !strings.ContainsAny(class, "*?[") {
		class += "*"
	}
	return class, nil
}
