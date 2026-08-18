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
