package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/t0k0sh1/cyclops/internal/config"
)

func Select(start string) (Backend, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, usageDiagnostic(fmt.Errorf("resolve working directory: %w", err))
	}
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	roots := findProjectRoots(dir)
	configured, configPath, found, err := config.Load(dir)
	if err != nil {
		return nil, usageDiagnostic(err)
	}
	if found {
		switch configured.Backend {
		case "gremlins":
			return Gremlins{}, nil
		case "cargo-mutants":
			if roots.cargo == "" {
				return nil, usageDiagnostic(fmt.Errorf(
					"backend %q configured in %s requires a Cargo.toml in or above the working directory",
					configured.Backend, configPath,
				))
			}
			return CargoMutants{Root: roots.cargo}, nil
		case "stryker-js":
			if roots.node == "" {
				return nil, usageDiagnostic(fmt.Errorf(
					"backend %q configured in %s requires a package.json in or above the working directory",
					configured.Backend, configPath,
				))
			}
			return StrykerJS{Root: roots.node}, nil
		case "pit":
			if roots.maven == "" {
				return nil, usageDiagnostic(fmt.Errorf(
					"backend %q configured in %s requires a pom.xml in or above the working directory",
					configured.Backend, configPath,
				))
			}
			return PIT{Root: roots.maven}, nil
		case "mutmut":
			if roots.python == "" {
				return nil, usageDiagnostic(fmt.Errorf(
					"backend %q configured in %s requires pyproject.toml, setup.cfg, or setup.py in or above the working directory",
					configured.Backend, configPath,
				))
			}
			return Mutmut{Root: roots.python}, nil
		default:
			return nil, usageDiagnostic(fmt.Errorf(
				"unknown backend %q in %s (supported: gremlins, cargo-mutants, stryker-js, pit, mutmut)",
				configured.Backend, configPath,
			))
		}
	}

	detected := []string{}
	if roots.goModule != "" {
		detected = append(detected, fmt.Sprintf("Go module at %s", roots.goModule))
	}
	if roots.cargo != "" {
		detected = append(detected, fmt.Sprintf("Cargo project at %s", roots.cargo))
	}
	if roots.stryker != "" {
		detected = append(detected, fmt.Sprintf("StrykerJS project at %s", roots.stryker))
	}
	if roots.pit != "" {
		detected = append(detected, fmt.Sprintf("PIT project at %s", roots.pit))
	}
	if roots.mutmut != "" {
		detected = append(detected, fmt.Sprintf("mutmut project at %s", roots.mutmut))
	}
	if len(detected) > 1 {
		return nil, usageDiagnostic(fmt.Errorf(
			"ambiguous mutation backend: found %s",
			strings.Join(detected, ", "),
		))
	}
	if roots.cargo != "" {
		return CargoMutants{Root: roots.cargo}, nil
	}
	if roots.stryker != "" {
		return StrykerJS{Root: roots.stryker}, nil
	}
	if roots.pit != "" {
		return PIT{Root: roots.pit}, nil
	}
	if roots.mutmut != "" {
		return Mutmut{Root: roots.mutmut}, nil
	}
	return Gremlins{}, nil
}

type projectRoots struct {
	goModule string
	cargo    string
	node     string
	stryker  string
	maven    string
	pit      string
	python   string
	mutmut   string
}

func findProjectRoots(dir string) projectRoots {
	var roots projectRoots
	for current := dir; ; current = filepath.Dir(current) {
		if roots.goModule == "" && regularFile(filepath.Join(current, "go.mod")) {
			roots.goModule = current
		}
		if roots.cargo == "" && regularFile(filepath.Join(current, "Cargo.toml")) {
			roots.cargo = current
		}
		packagePath := filepath.Join(current, "package.json")
		if roots.node == "" && regularFile(packagePath) {
			roots.node = current
		}
		if roots.stryker == "" && strykerProject(current, packagePath) {
			roots.stryker = current
		}
		pomPath := filepath.Join(current, "pom.xml")
		if roots.maven == "" && regularFile(pomPath) {
			roots.maven = current
		}
		if roots.pit == "" && pitProject(pomPath) {
			roots.pit = current
		}
		if roots.python == "" && pythonProject(current) {
			roots.python = current
		}
		if roots.mutmut == "" && mutmutProject(current) {
			roots.mutmut = current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return roots
}

func pythonProject(dir string) bool {
	for _, name := range []string{"pyproject.toml", "setup.cfg", "setup.py"} {
		if regularFile(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

func mutmutProject(dir string) bool {
	for _, candidate := range []struct {
		name, marker string
	}{
		{name: "pyproject.toml", marker: "[tool.mutmut]"},
		{name: "setup.cfg", marker: "[mutmut]"},
	} {
		contents, err := os.ReadFile(filepath.Join(dir, candidate.name))
		if err == nil && strings.Contains(string(contents), candidate.marker) {
			return true
		}
	}
	return false
}

func pitProject(pomPath string) bool {
	contents, err := os.ReadFile(pomPath)
	if err != nil {
		return false
	}
	return bytesContains(contents, []byte("org.pitest")) || bytesContains(contents, []byte("pitest-maven"))
}

func bytesContains(contents, fragment []byte) bool {
	return strings.Contains(string(contents), string(fragment))
}

func strykerProject(dir, packagePath string) bool {
	if findStrykerConfig(dir) != "" {
		return true
	}
	contents, err := os.ReadFile(packagePath)
	if err != nil {
		return false
	}
	var manifest struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
		Stryker         json.RawMessage            `json:"stryker"`
	}
	if json.Unmarshal(contents, &manifest) != nil {
		return false
	}
	if len(manifest.Stryker) > 0 {
		return true
	}
	for _, dependencies := range []map[string]json.RawMessage{manifest.Dependencies, manifest.DevDependencies} {
		if _, ok := dependencies["@stryker-mutator/core"]; ok {
			return true
		}
	}
	return false
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
