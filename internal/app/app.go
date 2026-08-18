package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	exitFailure = 1
	exitUsage   = 2
)

type targetSelection struct {
	targets    map[string]struct{}
	moduleRoot string
	scanRoot   string
}

// Run executes Gremlins mutation testing and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	list, inputs, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "cyclops: %v\n", err)
		return exitUsage
	}

	if list {
		if err := printTargets(inputs, stdout); err != nil {
			fmt.Fprintf(stderr, "cyclops: %v\n", err)
			return exitUsage
		}
		return 0
	}

	gremlinsArgs := []string{"unleash"}
	if len(inputs) > 0 {
		targets, err := expandTargets(inputs)
		if err != nil {
			fmt.Fprintf(stderr, "cyclops: %v\n", err)
			return exitUsage
		}
		gremlinsArgs, err = fileMutationArgs(targets)
		if err != nil {
			fmt.Fprintf(stderr, "cyclops: %v\n", err)
			return exitUsage
		}
	}

	gremlins, err := exec.LookPath("gremlins")
	if err != nil {
		fmt.Fprintln(stderr, "cyclops: gremlins was not found in PATH")
		fmt.Fprintln(stderr, "install it with: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest")
		return exitFailure
	}

	cmd := exec.Command(gremlins, gremlinsArgs...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "cyclops: failed to run gremlins: %v\n", err)
		return exitFailure
	}

	return 0
}

func parseArgs(args []string) (list bool, targets []string, err error) {
	options := true
	for _, arg := range args {
		if options && arg == "--" {
			options = false
			continue
		}
		if options && arg == "--list" {
			list = true
			continue
		}
		if options && strings.HasPrefix(arg, "-") {
			return false, nil, fmt.Errorf("unknown option: %s", arg)
		}
		targets = append(targets, arg)
	}
	return list, targets, nil
}

func printTargets(inputs []string, stdout io.Writer) error {
	var targets []string
	if len(inputs) == 0 {
		var err error
		targets, err = targetsBelowCurrentDirectory()
		if err != nil {
			return err
		}
	} else {
		var err error
		targets, err = expandTargets(inputs)
		if err != nil {
			return err
		}
		selection, err := resolveTargets(targets)
		if err != nil {
			return err
		}
		targets = targetsFromSelection(selection)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
	for _, target := range targets {
		rel, err := filepath.Rel(cwd, target)
		if err != nil {
			return fmt.Errorf("make target path relative: %w", err)
		}
		fmt.Fprintln(stdout, filepath.ToSlash(rel))
	}
	return nil
}

func targetsBelowCurrentDirectory() ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current directory: %w", err)
	}
	if _, err := findModuleRoot(cwd); err != nil {
		return nil, err
	}

	var targets []string
	err = filepath.WalkDir(cwd, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		targets = append(targets, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan current directory: %w", err)
	}
	sort.Strings(targets)
	return targets, nil
}

func expandTargets(inputs []string) ([]string, error) {
	targets := make(map[string]struct{})
	for _, input := range inputs {
		if _, err := os.Stat(input); err == nil || !hasGlobMeta(input) {
			targets[input] = struct{}{}
			continue
		}

		matches, err := doublestar.FilepathGlob(
			input,
			doublestar.WithFilesOnly(),
			doublestar.WithNoFollow(),
			doublestar.WithNoHidden(),
			doublestar.WithFailOnIOErrors(),
		)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", input, err)
		}

		matchedTargets := 0
		for _, match := range matches {
			if filepath.Ext(match) != ".go" || strings.HasSuffix(match, "_test.go") {
				continue
			}
			targets[match] = struct{}{}
			matchedTargets++
		}
		if matchedTargets == 0 {
			return nil, fmt.Errorf("pattern %q matched no Go source files", input)
		}
	}

	expanded := make([]string, 0, len(targets))
	for target := range targets {
		expanded = append(expanded, target)
	}
	sort.Strings(expanded)
	return expanded, nil
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[{")
}

func fileMutationArgs(targets []string) ([]string, error) {
	selection, err := resolveTargets(targets)
	if err != nil {
		return nil, err
	}
	gremlinsArgs := []string{"unleash", selection.scanRoot}

	err = filepath.WalkDir(selection.scanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if _, ok := selection.targets[absolutePath]; ok {
			return nil
		}

		rel, err := filepath.Rel(selection.scanRoot, path)
		if err != nil {
			return err
		}
		pattern := "^" + regexp.QuoteMeta(filepath.ToSlash(rel)) + "$"
		gremlinsArgs = append(gremlinsArgs, "--exclude-files", pattern)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan target package: %w", err)
	}

	return gremlinsArgs, nil
}

func resolveTargets(targets []string) (targetSelection, error) {
	selection := targetSelection{targets: make(map[string]struct{}, len(targets))}
	for _, target := range targets {
		absoluteTarget, root, err := validateTarget(target)
		if err != nil {
			return targetSelection{}, err
		}
		if selection.moduleRoot == "" {
			selection.moduleRoot = root
		} else if root != selection.moduleRoot {
			return targetSelection{}, fmt.Errorf("targets belong to different Go modules: %s and %s", selection.moduleRoot, root)
		}

		selection.targets[absoluteTarget] = struct{}{}
		if selection.scanRoot == "" {
			selection.scanRoot = filepath.Dir(absoluteTarget)
		} else {
			selection.scanRoot = commonDirectory(selection.scanRoot, filepath.Dir(absoluteTarget))
		}
	}
	return selection, nil
}

func targetsFromSelection(selection targetSelection) []string {
	targets := make([]string, 0, len(selection.targets))
	for target := range selection.targets {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

func validateTarget(target string) (absoluteTarget, moduleRoot string, err error) {
	if filepath.Ext(target) != ".go" {
		return "", "", fmt.Errorf("target must be a .go file: %s", target)
	}
	if strings.HasSuffix(target, "_test.go") {
		return "", "", fmt.Errorf("test files cannot be mutation targets: %s", target)
	}

	info, err := os.Stat(target)
	if err != nil {
		return "", "", fmt.Errorf("cannot access target %s: %w", target, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("target is not a regular file: %s", target)
	}

	absoluteTarget, err = filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("resolve target %s: %w", target, err)
	}
	moduleRoot, err = findModuleRoot(filepath.Dir(absoluteTarget))
	if err != nil {
		return "", "", err
	}
	return absoluteTarget, moduleRoot, nil
}

func findModuleRoot(dir string) (string, error) {
	for {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && info.Mode().IsRegular() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("target is not inside a Go module")
		}
		dir = parent
	}
}

func commonDirectory(left, right string) string {
	for {
		rel, err := filepath.Rel(left, right)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return left
		}
		parent := filepath.Dir(left)
		if parent == left {
			return left
		}
		left = parent
	}
}
