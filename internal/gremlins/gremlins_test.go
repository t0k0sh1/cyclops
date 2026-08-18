package gremlins

import (
	"regexp"
	"strings"
	"testing"
)

func TestBuildExcludePatternsCombinesAndEscapesPaths(t *testing.T) {
	paths := []string{`generated/file\.go`, `pkg/a\+b\.go`, `pkg/name\[1\]\.go`}
	patterns := buildExcludePatterns(paths)
	if len(patterns) != 1 {
		t.Fatalf("pattern count = %d, want 1", len(patterns))
	}

	rule, err := regexp.Compile(patterns[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"generated/file.go", "pkg/a+b.go", "pkg/name[1].go"} {
		if !rule.MatchString(path) {
			t.Errorf("pattern %q does not match %q", patterns[0], path)
		}
	}
	for _, path := range []string{"generated/fileXgo", "pkg/axb.go", "pkg/name1.go"} {
		if rule.MatchString(path) {
			t.Errorf("pattern %q unexpectedly matches %q", patterns[0], path)
		}
	}
}

func TestBuildExcludePatternsChunksLargeInput(t *testing.T) {
	path := strings.Repeat("a", 1024)
	paths := make([]string, 100)
	for index := range paths {
		paths[index] = path
	}

	patterns := buildExcludePatterns(paths)
	if len(patterns) < 2 {
		t.Fatalf("pattern count = %d, want at least 2", len(patterns))
	}
	for _, pattern := range patterns {
		if len(pattern) > maxExcludePatternBytes {
			t.Errorf("pattern size = %d, maximum = %d", len(pattern), maxExcludePatternBytes)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			t.Errorf("invalid pattern: %v", err)
		}
	}
}
