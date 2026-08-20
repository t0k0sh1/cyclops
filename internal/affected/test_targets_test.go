package affected

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTestTargetsMapsChangedTestsToPackageProductionFiles(t *testing.T) {
	repository := newRepository(t, map[string]string{
		"go.mod":              "module example.com/project\n\ngo 1.23\n",
		"service/add.go":      "package service\nfunc Add(a, b int) int { return a + b }\n",
		"service/other.go":    "package service\nfunc Other() {}\n",
		"service/add_test.go": "package service\n",
	})
	base := commitAll(t, repository, "base")
	writeFile(t, repository, "service/add_test.go", "package service\n// changed\n")

	targets, err := TestTargets(repository, base)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"service/add.go", "service/other.go"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("TestTargets() = %v, want %v", targets, want)
	}
}

func TestTestTargetsDeduplicatesPackageForMultipleTests(t *testing.T) {
	repository := newRepository(t, map[string]string{
		"go.mod":              "module example.com/project\n\ngo 1.23\n",
		"service/add.go":      "package service\nfunc Add() {}\n",
		"service/add_test.go": "package service\n",
		"service/sub_test.go": "package service\n",
	})
	base := commitAll(t, repository, "base")
	writeFile(t, repository, "service/add_test.go", "package service\n// changed\n")
	writeFile(t, repository, "service/sub_test.go", "package service\n// changed\n")

	targets, err := TestTargets(repository, base)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"service/add.go"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("TestTargets() = %v, want %v", targets, want)
	}
}

func TestTestTargetsReturnsEmptyWithoutProductionFiles(t *testing.T) {
	repository := newRepository(t, map[string]string{
		"go.mod":              "module example.com/project\n\ngo 1.23\n",
		"service/add_test.go": "package service\n",
	})
	base := commitAll(t, repository, "base")
	writeFile(t, repository, "service/add_test.go", "package service\n// changed\n")

	targets, err := TestTargets(repository, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("TestTargets() = %v, want no targets", targets)
	}
}

func TestTestTargetsMapsDeletedTestToRemainingPackage(t *testing.T) {
	repository := newRepository(t, map[string]string{
		"go.mod":              "module example.com/project\n\ngo 1.23\n",
		"service/add.go":      "package service\nfunc Add() {}\n",
		"service/add_test.go": "package service\n",
	})
	base := commitAll(t, repository, "base")
	if err := os.Remove(filepath.Join(repository, "service", "add_test.go")); err != nil {
		t.Fatal(err)
	}

	targets, err := TestTargets(repository, base)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"service/add.go"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("TestTargets() = %v, want %v", targets, want)
	}
}

func TestTestTargetsMapsBothPackagesAfterTestRename(t *testing.T) {
	repository := newRepository(t, map[string]string{
		"go.mod":            "module example.com/project\n\ngo 1.23\n",
		"old/old.go":        "package old\nfunc Old() {}\n",
		"old/value_test.go": "package old\n",
		"new/new.go":        "package new\nfunc New() {}\n",
	})
	base := commitAll(t, repository, "base")
	if err := os.Rename(
		filepath.Join(repository, "old", "value_test.go"),
		filepath.Join(repository, "new", "value_test.go"),
	); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repository, "new/value_test.go", "package new\n")
	runGit(t, repository, "add", "--all")

	targets, err := TestTargets(repository, base)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"new/new.go", "old/old.go"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("TestTargets() = %v, want %v", targets, want)
	}
}

func TestChangedTestDirectoriesIncludesBothSidesOfRename(t *testing.T) {
	input := []byte("R100\x00old/a_test.go\x00new/a_test.go\x00")
	got, err := changedTestDirectories(input)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"new", "old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changedTestDirectories() = %v, want %v", got, want)
	}
}

func TestTestTargetsMapsRustIntegrationTestToPackageSources(t *testing.T) {
	requireCargo(t)
	repository := newRepository(t, map[string]string{
		"Cargo.toml":   "[package]\nname = \"example\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
		"src/lib.rs":   "pub mod value;\n",
		"src/value.rs": "pub fn value() -> usize { 1 }\n",
		"tests/api.rs": "#[test]\nfn api() {}\n",
	})
	base := commitAll(t, repository, "base")
	writeFile(t, repository, "tests/api.rs", "#[test]\nfn api() { assert!(true); }\n")

	targets, err := TestTargets(repository, base)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/lib.rs", "src/value.rs"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("TestTargets() = %v, want %v", targets, want)
	}
}

func TestTestTargetsMapsCustomCargoTestTarget(t *testing.T) {
	requireCargo(t)
	repository := newRepository(t, map[string]string{
		"Cargo.toml":    "[package]\nname = \"example\"\nversion = \"0.1.0\"\nedition = \"2021\"\n\n[[test]]\nname = \"check\"\npath = \"spec/check.rs\"\n",
		"src/lib.rs":    "pub fn value() -> usize { 1 }\n",
		"spec/check.rs": "#[test]\nfn check() {}\n",
	})
	base := commitAll(t, repository, "base")
	writeFile(t, repository, "spec/check.rs", "#[test]\nfn check() { assert!(true); }\n")

	targets, err := TestTargets(repository, base)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"src/lib.rs"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("TestTargets() = %v, want %v", targets, want)
	}
}

func TestTestTargetsLimitsRustWorkspaceTestToOwningPackage(t *testing.T) {
	requireCargo(t)
	repository := newRepository(t, map[string]string{
		"Cargo.toml":               "[workspace]\nmembers = [\"alpha\", \"beta\"]\nresolver = \"2\"\n",
		"alpha/Cargo.toml":         "[package]\nname = \"alpha\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
		"alpha/src/lib.rs":         "pub fn alpha() {}\n",
		"alpha/tests/alpha.rs":     "#[test]\nfn alpha() {}\n",
		"beta/Cargo.toml":          "[package]\nname = \"beta\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
		"beta/src/lib.rs":          "pub fn beta() {}\n",
		"beta/src/nested/value.rs": "pub fn value() {}\n",
	})
	base := commitAll(t, repository, "base")
	writeFile(t, repository, "alpha/tests/alpha.rs", "#[test]\nfn alpha() { assert!(true); }\n")

	targets, err := TestTargets(repository, base)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha/src/lib.rs"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("TestTargets() = %v, want %v", targets, want)
	}
}

func requireCargo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo is not installed")
	}
}

func newRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.name", "Cyclops Test")
	runGit(t, root, "config", "user.email", "cyclops@example.invalid")
	for path, contents := range files {
		writeFile(t, root, path, contents)
	}
	return root
}

func writeFile(t *testing.T, root, path, contents string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitAll(t *testing.T, root, message string) string {
	t.Helper()
	runGit(t, root, "add", "--all")
	runGit(t, root, "commit", "-qm", message)
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(output[:len(output)-1])
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
