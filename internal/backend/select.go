package backend

import (
	"fmt"
	"os"
	"path/filepath"

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

	goRoot, cargoRoot := projectRoots(dir)
	configured, configPath, found, err := config.Load(dir)
	if err != nil {
		return nil, usageDiagnostic(err)
	}
	if found {
		switch configured.Backend {
		case "gremlins":
			return Gremlins{}, nil
		case "cargo-mutants":
			if cargoRoot == "" {
				return nil, usageDiagnostic(fmt.Errorf(
					"backend %q configured in %s requires a Cargo.toml in or above the working directory",
					configured.Backend, configPath,
				))
			}
			return CargoMutants{Root: cargoRoot}, nil
		default:
			return nil, usageDiagnostic(fmt.Errorf(
				"unknown backend %q in %s (supported: gremlins, cargo-mutants)",
				configured.Backend, configPath,
			))
		}
	}

	if goRoot != "" && cargoRoot != "" {
		return nil, usageDiagnostic(fmt.Errorf(
			"ambiguous mutation backend: found Go module at %s and Cargo project at %s",
			goRoot, cargoRoot,
		))
	}
	if cargoRoot != "" {
		return CargoMutants{Root: cargoRoot}, nil
	}
	return Gremlins{}, nil
}

func projectRoots(dir string) (string, string) {
	var goRoot, cargoRoot string
	for current := dir; ; current = filepath.Dir(current) {
		if goRoot == "" && regularFile(filepath.Join(current, "go.mod")) {
			goRoot = current
		}
		if cargoRoot == "" && regularFile(filepath.Join(current, "Cargo.toml")) {
			cargoRoot = current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return goRoot, cargoRoot
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
