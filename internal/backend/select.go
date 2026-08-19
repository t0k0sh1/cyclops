package backend

import (
	"fmt"
	"os"
	"path/filepath"
)

func Select(start string) (Backend, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, usageDiagnostic(fmt.Errorf("resolve working directory: %w", err))
	}
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}

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

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
