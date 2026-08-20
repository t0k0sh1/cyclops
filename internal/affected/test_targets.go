package affected

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type goPackage struct {
	Dir      string
	GoFiles  []string
	CgoFiles []string
}

func TestTargets(root, base string) ([]string, error) {
	if base == "" || strings.HasPrefix(base, "-") {
		return nil, fmt.Errorf("invalid diff reference: %s", base)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	cmd := exec.Command("git", "diff", "--name-status", "-z", "--find-renames", base, "--", "*.go")
	cmd.Dir = absoluteRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list changed Go files from %s: %s", base, commandError(err, stderr.String()))
	}

	directories, err := changedTestDirectories(output)
	if err != nil {
		return nil, err
	}
	targets := make(map[string]struct{})
	for _, relativeDir := range directories {
		directory := filepath.Join(absoluteRoot, filepath.FromSlash(relativeDir))
		info, err := os.Stat(directory)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("inspect changed test package %s: %w", relativeDir, err)
		}
		if !info.IsDir() {
			continue
		}
		pkg, err := listPackage(directory)
		if err != nil {
			return nil, fmt.Errorf("resolve changed test package %s: %w", relativeDir, err)
		}
		for _, name := range append(pkg.GoFiles, pkg.CgoFiles...) {
			absolute := filepath.Join(pkg.Dir, name)
			relative, err := filepath.Rel(absoluteRoot, absolute)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("production target is outside repository: %s", absolute)
			}
			targets[filepath.ToSlash(relative)] = struct{}{}
		}
	}

	result := make([]string, 0, len(targets))
	for target := range targets {
		result = append(result, target)
	}
	sort.Strings(result)
	return result, nil
}

func changedTestDirectories(output []byte) ([]string, error) {
	fields := bytes.Split(output, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	directories := make(map[string]struct{})
	for index := 0; index < len(fields); {
		status := string(fields[index])
		index++
		pathCount := 1
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			pathCount = 2
		}
		if index+pathCount > len(fields) {
			return nil, fmt.Errorf("decode git diff: status %q has incomplete paths", status)
		}
		for _, raw := range fields[index : index+pathCount] {
			path := filepath.ToSlash(string(raw))
			if strings.HasSuffix(path, "_test.go") {
				directory := filepath.ToSlash(filepath.Dir(path))
				if directory == "." {
					directory = ""
				}
				directories[directory] = struct{}{}
			}
		}
		index += pathCount
	}
	result := make([]string, 0, len(directories))
	for directory := range directories {
		result = append(result, directory)
	}
	sort.Strings(result)
	return result, nil
}

func listPackage(directory string) (goPackage, error) {
	cmd := exec.Command("go", "list", "-json", ".")
	cmd.Dir = directory
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return goPackage{}, fmt.Errorf("go list: %s", commandError(err, stderr.String()))
	}
	var pkg goPackage
	if err := json.Unmarshal(output, &pkg); err != nil {
		return goPackage{}, fmt.Errorf("decode go list output: %w", err)
	}
	return pkg, nil
}

func commandError(err error, stderr string) string {
	detail := strings.TrimSpace(stderr)
	if detail != "" {
		return detail
	}
	return err.Error()
}
