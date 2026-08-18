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

// Run executes Gremlins mutation testing and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	gremlinsArgs := []string{"unleash"}
	if len(args) > 0 {
		targets, err := expandTargets(args)
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
	selected := make(map[string]struct{}, len(targets))
	var moduleRoot string
	var scanRoot string

	for _, target := range targets {
		absoluteTarget, root, err := validateTarget(target)
		if err != nil {
			return nil, err
		}
		if moduleRoot == "" {
			moduleRoot = root
		} else if root != moduleRoot {
			return nil, fmt.Errorf("targets belong to different Go modules: %s and %s", moduleRoot, root)
		}

		selected[absoluteTarget] = struct{}{}
		if scanRoot == "" {
			scanRoot = filepath.Dir(absoluteTarget)
		} else {
			scanRoot = commonDirectory(scanRoot, filepath.Dir(absoluteTarget))
		}
	}

	gremlinsArgs := []string{"unleash", scanRoot}

	err := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
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
		if _, ok := selected[absolutePath]; ok {
			return nil
		}

		rel, err := filepath.Rel(scanRoot, path)
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
