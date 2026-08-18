package target

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type Selection struct {
	Files      map[string]struct{}
	ModuleRoot string
	ScanRoot   string
}

func Expand(inputs []string) ([]string, error) {
	files := make(map[string]struct{})
	for _, input := range inputs {
		if _, err := os.Stat(input); err == nil || !strings.ContainsAny(input, "*?[{") {
			files[input] = struct{}{}
			continue
		}
		matches, err := doublestar.FilepathGlob(input, doublestar.WithFilesOnly(), doublestar.WithNoFollow(), doublestar.WithNoHidden(), doublestar.WithFailOnIOErrors())
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", input, err)
		}
		matched := 0
		for _, match := range matches {
			if filepath.Ext(match) == ".go" && !strings.HasSuffix(match, "_test.go") {
				files[match] = struct{}{}
				matched++
			}
		}
		if matched == 0 {
			return nil, fmt.Errorf("pattern %q matched no Go source files", input)
		}
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func Resolve(paths []string) (Selection, error) {
	selection := Selection{Files: make(map[string]struct{}, len(paths))}
	for _, path := range paths {
		absolute, root, err := validate(path)
		if err != nil {
			return Selection{}, err
		}
		if selection.ModuleRoot == "" {
			selection.ModuleRoot = root
		} else if root != selection.ModuleRoot {
			return Selection{}, fmt.Errorf("targets belong to different Go modules: %s and %s", selection.ModuleRoot, root)
		}
		selection.Files[absolute] = struct{}{}
		if selection.ScanRoot == "" {
			selection.ScanRoot = filepath.Dir(absolute)
		} else {
			selection.ScanRoot = commonDirectory(selection.ScanRoot, filepath.Dir(absolute))
		}
	}
	return selection, nil
}

func (s Selection) Paths() []string {
	paths := make([]string, 0, len(s.Files))
	for path := range s.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func BelowCurrentDirectory() ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current directory: %w", err)
	}
	if _, err := findModuleRoot(cwd); err != nil {
		return nil, err
	}
	var paths []string
	err = filepath.WalkDir(cwd, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan current directory: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func validate(path string) (absolute, moduleRoot string, err error) {
	if filepath.Ext(path) != ".go" {
		return "", "", fmt.Errorf("target must be a .go file: %s", path)
	}
	if strings.HasSuffix(path, "_test.go") {
		return "", "", fmt.Errorf("test files cannot be mutation targets: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("cannot access target %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("target is not a regular file: %s", path)
	}
	absolute, err = filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve target %s: %w", path, err)
	}
	moduleRoot, err = findModuleRoot(filepath.Dir(absolute))
	return absolute, moduleRoot, err
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
