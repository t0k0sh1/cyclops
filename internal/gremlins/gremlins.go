package gremlins

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/t0k0sh1/cyclops/internal/target"
)

var ErrNotFound = errors.New("gremlins executable not found")

const maxExcludePatternBytes = 32 * 1024

type Request struct {
	Selection *target.Selection
	DryRun    bool
	DiffBase  string
	Output    string
}

func Arguments(request Request) ([]string, error) {
	args := []string{"unleash"}
	selection := request.Selection
	if selection != nil {
		args = append(args, selection.ScanRoot)
		var excluded []string
		err := filepath.WalkDir(selection.ScanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if _, ok := selection.Files[absolute]; ok {
				return nil
			}
			rel, err := filepath.Rel(selection.ScanRoot, path)
			if err != nil {
				return err
			}
			excluded = append(excluded, regexp.QuoteMeta(filepath.ToSlash(rel)))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan target package: %w", err)
		}
		for _, pattern := range buildExcludePatterns(excluded) {
			args = append(args, "--exclude-files", pattern)
		}
	}
	if request.DiffBase != "" {
		args = append(args, "--diff", request.DiffBase)
	}
	if request.DryRun {
		args = append(args, "--dry-run")
	}
	if request.Output != "" {
		args = append(args, "--output", request.Output)
	}
	return args, nil
}

func buildExcludePatterns(paths []string) []string {
	const framingBytes = len("^()$")
	var patterns []string
	var chunk []string
	chunkBytes := framingBytes

	flush := func() {
		if len(chunk) == 0 {
			return
		}
		patterns = append(patterns, "^("+strings.Join(chunk, "|")+")$")
		chunk = nil
		chunkBytes = framingBytes
	}

	for _, path := range paths {
		additionalBytes := len(path)
		if len(chunk) > 0 {
			additionalBytes++ // Alternation separator.
		}
		if len(chunk) > 0 && chunkBytes+additionalBytes > maxExcludePatternBytes {
			flush()
			additionalBytes = len(path)
		}
		chunk = append(chunk, path)
		chunkBytes += additionalBytes
	}
	flush()
	return patterns
}

func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	path, err := exec.LookPath("gremlins")
	if err != nil {
		return ErrNotFound
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return cmd.Run()
}
