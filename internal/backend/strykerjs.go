package backend

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	mutationresult "github.com/t0k0sh1/cyclops/internal/result"
)

var strykerHunk = regexp.MustCompile(`^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

type StrykerJS struct {
	Root string
}

func (StrykerJS) ID() string { return "stryker-js" }

func (StrykerJS) Capabilities() Capabilities {
	return Capabilities{Diff: true}
}

func (backend StrykerJS) Run(request Request) (mutationresult.Run, error) {
	run := normalizedRun(backend.ID(), request)
	targets, err := backend.mutationTargets(request.Targets, request.DiffBase)
	if err != nil {
		return run, usageDiagnostic(err)
	}
	if request.DiffBase != "" && len(targets) == 0 {
		run.State = mutationresult.StateNoCandidates
		return run, nil
	}

	path, err := backend.executable()
	if err != nil {
		recordExecutionError(&run, "tool", err.Error(), nil)
		return run, &Diagnostic{
			ExitCode: ExitFailure,
			Lines: []string{
				"cyclops: StrykerJS was not found in node_modules/.bin or PATH",
				"install it with: npm install --save-dev @stryker-mutator/core",
			},
			Err: err,
		}
	}
	output, err := os.CreateTemp("", "cyclops-stryker-js-*.json")
	if err != nil {
		return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: create StrykerJS result file: %v", err)}, Err: err}
	}
	outputPath := output.Name()
	if err := output.Close(); err != nil {
		os.Remove(outputPath)
		return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: close StrykerJS result file: %v", err)}, Err: err}
	}
	defer os.Remove(outputPath)
	// Stryker refuses to overwrite an existing report in some versions.
	if err := os.Remove(outputPath); err != nil {
		return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: prepare StrykerJS result file: %v", err)}, Err: err}
	}

	configPath, err := backend.writeConfig(outputPath)
	if err != nil {
		return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: create StrykerJS adapter configuration: %v", err)}, Err: err}
	}
	defer os.Remove(configPath)
	args := []string{"run", "--reporters", "json"}
	if len(targets) > 0 {
		args = append(args, "--mutate", strings.Join(targets, ","))
	}
	args = append(args, configPath)
	cmd := exec.Command(path, args...)
	cmd.Dir = backend.Root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = request.Stdin, request.Stdout, request.Stderr
	executionErr := cmd.Run()
	parsed, parseErr := parseStrykerFile(outputPath, request)
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
		return run, &ProcessExit{BackendID: backend.ID(), Code: code, Meaning: "StrykerJS failed or mutation threshold was not met", Err: executionErr}
	}
	recordExecutionError(&run, "tool", executionErr.Error(), nil)
	return run, &Diagnostic{ExitCode: ExitFailure, Lines: []string{fmt.Sprintf("cyclops: failed to run StrykerJS: %v", executionErr)}, Err: executionErr}
}

func (backend StrykerJS) writeConfig(outputPath string) (string, error) {
	base := "{}"
	configPath := findStrykerConfig(backend.Root)
	if configPath != "" {
		if strings.HasSuffix(configPath, ".json") {
			contents, err := os.ReadFile(configPath)
			if err != nil {
				return "", err
			}
			var value any
			if err := json.Unmarshal(contents, &value); err != nil {
				return "", fmt.Errorf("parse %s: %w", configPath, err)
			}
			encoded, _ := json.Marshal(value)
			base = string(encoded)
		} else {
			base = "baseConfig"
		}
	} else if contents, err := os.ReadFile(filepath.Join(backend.Root, "package.json")); err == nil {
		var manifest struct {
			Stryker json.RawMessage `json:"stryker"`
		}
		if json.Unmarshal(contents, &manifest) == nil && len(manifest.Stryker) > 0 {
			base = string(manifest.Stryker)
		}
	}

	file, err := os.CreateTemp(backend.Root, ".cyclops-stryker-*.mjs")
	if err != nil {
		return "", err
	}
	name := file.Name()
	var source strings.Builder
	fmt.Fprintf(&source, "// cyclops-report: %s\n", outputPath)
	if configPath != "" && !strings.HasSuffix(configPath, ".json") {
		encodedImport, _ := json.Marshal("./" + filepath.ToSlash(filepath.Base(configPath)))
		fmt.Fprintf(&source, "import baseConfig from %s;\n", encodedImport)
	}
	encodedOutput, _ := json.Marshal(outputPath)
	fmt.Fprintf(&source, "export default { ...%s, reporters: ['json'], jsonReporter: { fileName: %s } };\n", base, encodedOutput)
	if _, err := file.WriteString(source.String()); err != nil {
		file.Close()
		os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

func findStrykerConfig(root string) string {
	for _, prefix := range []string{"stryker.conf", ".stryker.conf", "stryker.config", ".stryker.config"} {
		for _, extension := range []string{".json", ".js", ".mjs", ".cjs"} {
			path := filepath.Join(root, prefix+extension)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path
			}
		}
	}
	return ""
}

func (backend StrykerJS) executable() (string, error) {
	for _, name := range []string{"stryker", "stryker.cmd"} {
		path := filepath.Join(backend.Root, "node_modules", ".bin", name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return exec.LookPath("stryker")
}

func parseStrykerFile(path string, request Request) (mutationresult.Run, error) {
	file, err := os.Open(path)
	if err != nil {
		return mutationresult.Run{}, err
	}
	defer file.Close()
	return mutationresult.ParseStryker(file, mutationresult.Scope{
		DiffBase: request.DiffBase,
		Targets:  append([]string(nil), request.Targets...),
	})
}

func (backend StrykerJS) mutationTargets(inputs []string, diffBase string) ([]string, error) {
	patterns := make([]string, 0, len(inputs))
	for _, input := range inputs {
		pattern, err := backend.targetPattern(input)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
	}
	if diffBase == "" {
		return patterns, nil
	}
	ranges, err := backend.diffRanges(diffBase)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return ranges, nil
	}
	filtered := ranges[:0]
	for _, mutationRange := range ranges {
		file := mutationRange
		if colon := strings.LastIndex(file, ":"); colon >= 0 {
			file = file[:colon]
		}
		for _, pattern := range patterns {
			matched, matchErr := doublestar.Match(pattern, file)
			if matchErr != nil {
				return nil, fmt.Errorf("invalid target pattern %q: %w", pattern, matchErr)
			}
			if matched {
				filtered = append(filtered, mutationRange)
				break
			}
		}
	}
	return filtered, nil
}

func (backend StrykerJS) targetPattern(input string) (string, error) {
	absolute, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolve target %s: %w", input, err)
	}
	relative, err := filepath.Rel(backend.Root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("target is outside StrykerJS project %s: %s", backend.Root, input)
	}
	pattern := filepath.ToSlash(relative)
	if !strykerSource(pattern) {
		return "", fmt.Errorf("target must be a JavaScript, TypeScript, HTML, Vue, or Svelte source file or pattern: %s", input)
	}
	if !strings.ContainsAny(input, "*?[{") {
		info, statErr := os.Stat(absolute)
		if statErr != nil {
			return "", fmt.Errorf("cannot access target %s: %w", input, statErr)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("target is not a regular file: %s", input)
		}
	}
	return pattern, nil
}

func (backend StrykerJS) diffRanges(base string) ([]string, error) {
	if strings.HasPrefix(base, "-") {
		return nil, fmt.Errorf("invalid diff reference: %s", base)
	}
	cmd := exec.Command("git", "-c", "core.quotePath=false", "diff", "--no-ext-diff", "--unified=0", "--diff-filter=ACMRTUXB", base, "--")
	cmd.Dir = backend.Root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("generate diff from %s: %s", base, detail)
	}

	var file string
	ranges := make(map[string][]lineRange)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ b/") {
			file = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		match := strykerHunk.FindStringSubmatch(line)
		if file == "" || match == nil || !strykerSource(file) {
			continue
		}
		start, _ := strconv.Atoi(match[1])
		count := 1
		if match[2] != "" {
			count, _ = strconv.Atoi(match[2])
		}
		if count > 0 {
			ranges[file] = append(ranges[file], lineRange{start: start, end: start + count - 1})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read diff from %s: %w", base, err)
	}
	var targets []string
	for path, fileRanges := range ranges {
		for _, item := range mergeLineRanges(fileRanges) {
			targets = append(targets, fmt.Sprintf("%s:%d-%d", filepath.ToSlash(path), item.start, item.end))
		}
	}
	sort.Strings(targets)
	return targets, nil
}

type lineRange struct{ start, end int }

func mergeLineRanges(ranges []lineRange) []lineRange {
	if len(ranges) < 2 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	merged := []lineRange{ranges[0]}
	for _, current := range ranges[1:] {
		last := &merged[len(merged)-1]
		if current.start <= last.end+1 {
			if current.end > last.end {
				last.end = current.end
			}
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

func strykerSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".mts", ".cts", ".html", ".vue", ".svelte":
		return true
	default:
		return false
	}
}
