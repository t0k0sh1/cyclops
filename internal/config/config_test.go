package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFindsNearestConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("backend: cargo-mutants\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "src", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	loaded, path, found, err := Load(nested)
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Backend != "cargo-mutants" || path != filepath.Join(root, FileName) {
		t.Fatalf("Load() = %+v, %q, %t", loaded, path, found)
	}
}

func TestLoadReturnsNotFound(t *testing.T) {
	_, path, found, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if found || path != "" {
		t.Fatalf("Load() path = %q, found = %t", path, found)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("backend: gremlins\nbackedn: gremlins\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "field backedn not found") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRequiresBackend(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("# empty configuration\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "backend is required") {
		t.Fatalf("Load() error = %v", err)
	}
}
