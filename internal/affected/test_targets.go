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

type cargoMetadata struct {
	Packages []cargoPackage `json:"packages"`
}

type cargoPackage struct {
	ManifestPath string        `json:"manifest_path"`
	Targets      []cargoTarget `json:"targets"`
}

type cargoTarget struct {
	Kind    []string `json:"kind"`
	SrcPath string   `json:"src_path"`
}

func TestTargets(root, base string) ([]string, error) {
	if base == "" || strings.HasPrefix(base, "-") {
		return nil, fmt.Errorf("invalid diff reference: %s", base)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	absoluteRoot, err = filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	goRoot, cargoRoot, err := projectRoots(absoluteRoot)
	if err != nil {
		return nil, err
	}
	if cargoRoot != "" {
		return rustTestTargets(cargoRoot, base)
	}
	return goTestTargets(goRoot, base)
}

func goTestTargets(absoluteRoot, base string) ([]string, error) {
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

func rustTestTargets(root, base string) ([]string, error) {
	metadata, err := loadCargoMetadata(root)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "diff", "--name-status", "-z", "--find-renames", base, "--", "*.rs")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list changed Rust files from %s: %s", base, commandError(err, stderr.String()))
	}
	changed, err := changedPaths(output)
	if err != nil {
		return nil, err
	}

	packages := make(map[string]struct{})
	for _, path := range changed {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		for _, pkg := range metadata.Packages {
			packageRoot := filepath.Dir(pkg.ManifestPath)
			relative, relErr := filepath.Rel(packageRoot, absolute)
			if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				continue
			}
			if isCargoTestPath(relative, absolute, pkg.Targets) {
				packages[packageRoot] = struct{}{}
			}
		}
	}

	targets := make(map[string]struct{})
	for packageRoot := range packages {
		sourceRoot := filepath.Join(packageRoot, "src")
		if _, err := os.Stat(sourceRoot); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("inspect Rust package source %s: %w", sourceRoot, err)
		}
		err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".rs" {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return fmt.Errorf("Rust production target is outside project: %s", path)
			}
			targets[filepath.ToSlash(relative)] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("list Rust production targets for %s: %w", packageRoot, err)
		}
	}
	return sortedKeys(targets), nil
}

func loadCargoMetadata(root string) (cargoMetadata, error) {
	cmd := exec.Command("cargo", "metadata", "--format-version", "1", "--no-deps")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return cargoMetadata{}, fmt.Errorf("cargo metadata: %s", commandError(err, stderr.String()))
	}
	var metadata cargoMetadata
	if err := json.Unmarshal(output, &metadata); err != nil {
		return cargoMetadata{}, fmt.Errorf("decode cargo metadata: %w", err)
	}
	return metadata, nil
}

func isCargoTestPath(relative, absolute string, targets []cargoTarget) bool {
	relative = filepath.ToSlash(relative)
	if strings.HasPrefix(relative, "tests/") {
		return true
	}
	for _, target := range targets {
		if contains(target.Kind, "test") && samePath(target.SrcPath, absolute) {
			return true
		}
	}
	return false
}

func changedPaths(output []byte) ([]string, error) {
	fields := bytes.Split(output, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	paths := make(map[string]struct{})
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
			paths[filepath.ToSlash(string(raw))] = struct{}{}
		}
		index += pathCount
	}
	return sortedKeys(paths), nil
}

func projectRoots(start string) (goRoot, cargoRoot string, err error) {
	for directory := start; ; directory = filepath.Dir(directory) {
		if goRoot == "" && fileExists(filepath.Join(directory, "go.mod")) {
			goRoot = directory
		}
		if cargoRoot == "" && fileExists(filepath.Join(directory, "Cargo.toml")) {
			cargoRoot = directory
		}
		if filepath.Dir(directory) == directory {
			break
		}
	}
	if goRoot != "" && cargoRoot != "" {
		return "", "", fmt.Errorf("ambiguous project: found both go.mod and Cargo.toml")
	}
	if goRoot == "" && cargoRoot == "" {
		return "", "", fmt.Errorf("cannot find go.mod or Cargo.toml")
	}
	return goRoot, cargoRoot, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func samePath(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && leftAbsolute == rightAbsolute
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
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
