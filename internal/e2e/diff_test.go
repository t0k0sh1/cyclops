//go:build e2e

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiffDryRunUsesChangedLines(t *testing.T) {
	gremlins, err := exec.LookPath("gremlins")
	if err != nil {
		t.Fatal("gremlins must be installed and available in PATH")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("git must be installed and available in PATH")
	}

	projectRoot := cyclopsProjectRoot(t)
	testRoot := t.TempDir()
	binDir := t.TempDir()
	cyclops := filepath.Join(binDir, "cyclops")
	goCache := filepath.Join(t.TempDir(), "go-cache")
	environment := append(os.Environ(), "GOCACHE="+goCache)

	run(t, projectRoot, environment, "go", "build", "-o", cyclops, "./cmd/cyclops")
	writeFile(t, filepath.Join(testRoot, "go.mod"), "module example.com/cyclops-diff-e2e\n\ngo 1.23\n")
	writeFile(t, filepath.Join(testRoot, "arithmetic.go"), "package arithmetic\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n")
	writeFile(t, filepath.Join(testRoot, "arithmetic_test.go"), "package arithmetic\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif got := Add(1, 2); got != 3 {\n\t\tt.Fatalf(\"Add(1, 2) = %d, want 3\", got)\n\t}\n}\n")

	run(t, testRoot, environment, git, "init", "--quiet")
	run(t, testRoot, environment, git, "config", "user.name", "Cyclops E2E")
	run(t, testRoot, environment, git, "config", "user.email", "cyclops-e2e@example.invalid")
	run(t, testRoot, environment, git, "add", "go.mod", "arithmetic.go", "arithmetic_test.go")
	run(t, testRoot, environment, git, "commit", "--quiet", "-m", "initial")

	pathEnvironment := append(environment, "PATH="+filepath.Dir(gremlins)+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeFile(t, filepath.Join(testRoot, "arithmetic.go"), "package arithmetic\n\nfunc Add(a, b int) int {\n\treturn a + b // Sum both operands.\n}\n")
	withChanges := run(t, testRoot, pathEnvironment, cyclops, "--dry-run", "--diff", "HEAD")
	if !strings.Contains(withChanges, "RUNNABLE ARITHMETIC_BASE at arithmetic.go:4:11") {
		t.Fatalf("diff dry-run should select the mutant on the changed line:\n%s", withChanges)
	}
	if !strings.Contains(withChanges, "Runnable: 1") {
		t.Fatalf("diff dry-run should have exactly one runnable mutant:\n%s", withChanges)
	}
}

func cyclopsProjectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, environment []string, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}
