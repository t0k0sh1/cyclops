package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunStartsFullGremlinsMutationTest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}

	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	gremlins := filepath.Join(binDir, "gremlins")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CYCLOPS_TEST_ARGS_FILE\"\nprintf 'gremlins stdout\\n'\nprintf 'gremlins stderr\\n' >&2\n"
	if err := os.WriteFile(gremlins, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("CYCLOPS_TEST_ARGS_FILE", argsFile)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(nil, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", code, stderr.String())
	}

	gotArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(gotArgs), "unleash\n"; got != want {
		t.Fatalf("gremlins arguments = %q, want %q", got, want)
	}
	if got, want := stdout.String(), "gremlins stdout\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "gremlins stderr\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestRunReturnsGremlinsExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}

	binDir := t.TempDir()
	gremlins := filepath.Join(binDir, "gremlins")
	if err := os.WriteFile(gremlins, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	if code := Run(nil, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 7 {
		t.Fatalf("Run() exit code = %d, want 7", code)
	}
}

func TestRunTargetsOneGoFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}

	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	gremlins := filepath.Join(binDir, "gremlins")
	if err := os.WriteFile(gremlins, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CYCLOPS_TEST_ARGS_FILE\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("CYCLOPS_TEST_ARGS_FILE", argsFile)

	packageDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(packageDir, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(packageDir, "selected.go")
	other := filepath.Join(packageDir, "other.go")
	nestedDir := filepath.Join(packageDir, "nested")
	if err := os.Mkdir(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{target, other, filepath.Join(packageDir, "selected_test.go"), filepath.Join(nestedDir, "nested.go")} {
		if err := os.WriteFile(file, []byte("package example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stderr bytes.Buffer
	if code := Run([]string{target}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", code, stderr.String())
	}
	gotArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"unleash",
		packageDir,
		"--exclude-files",
		`^nested/nested\.go$`,
		"--exclude-files",
		`^other\.go$`,
	}, "\n") + "\n"
	if got := string(gotArgs); got != want {
		t.Fatalf("gremlins arguments = %q, want %q", got, want)
	}
}

func TestRunTargetsFilesAcrossPackages(t *testing.T) {
	moduleRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	firstDir := filepath.Join(moduleRoot, "first")
	secondDir := filepath.Join(moduleRoot, "second")
	for _, dir := range []string{firstDir, secondDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	first := filepath.Join(firstDir, "first.go")
	second := filepath.Join(secondDir, "second.go")
	excluded := filepath.Join(secondDir, "excluded.go")
	for _, file := range []string{first, second, excluded} {
		if err := os.WriteFile(file, []byte("package example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := fileMutationArgs([]string{first, second, first})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"unleash", moduleRoot, "--exclude-files", `^second/excluded\.go$`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}

func TestExpandTargetsSupportsBracesAndGlobstar(t *testing.T) {
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "foo", "bar", "bar.go"),
		filepath.Join(root, "foo", "bas", "bas.go"),
		filepath.Join(root, "foo", "bas", "nested", "nested.go"),
		filepath.Join(root, "foo", "bas", "nested", "nested_test.go"),
	}
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("package example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pattern := filepath.Join(root, "foo", "{bar,bas}", "**", "*.go")
	got, err := expandTargets([]string{pattern, files[0]})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{files[0], files[1], files[2]}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("targets = %q, want %q", got, want)
	}
}

func TestExpandTargetsRejectsPatternWithoutGoSourceFiles(t *testing.T) {
	root := t.TempDir()
	testFile := filepath.Join(root, "example_test.go")
	if err := os.WriteFile(testFile, []byte("package example\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := expandTargets([]string{filepath.Join(root, "*.go")})
	if err == nil || !strings.Contains(err.Error(), "matched no Go source files") {
		t.Fatalf("expandTargets() error = %v", err)
	}
}

func TestRunListsExpandedTargetsWithoutGremlins(t *testing.T) {
	moduleRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.go", "b.go", "b_test.go"} {
		if err := os.WriteFile(filepath.Join(moduleRoot, name), []byte("package example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdirForTest(t, moduleRoot)
	t.Setenv("PATH", t.TempDir())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run([]string{"--list", "*.go"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "a.go\nb.go\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunListsAllTargetsFromCurrentDirectory(t *testing.T) {
	moduleRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(moduleRoot, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root.go", filepath.Join("nested", "nested.go")} {
		if err := os.WriteFile(filepath.Join(moduleRoot, name), []byte("package example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdirForTest(t, moduleRoot)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run([]string{"--list"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "nested/nested.go\nroot.go\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunRejectsTestFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "example_test.go")
	if err := os.WriteFile(target, []byte("package example\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if code := Run([]string{target}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code != exitUsage {
		t.Fatalf("Run() exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "test files cannot be mutation targets") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}
