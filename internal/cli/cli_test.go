package cli

import "testing"

func TestParseListTestTargets(t *testing.T) {
	options, err := Parse([]string{"--list-test-targets", "origin/main"})
	if err != nil {
		t.Fatal(err)
	}
	if options.ListTestTargets != "origin/main" {
		t.Fatalf("ListTestTargets = %q, want origin/main", options.ListTestTargets)
	}
}

func TestParseListTestTargetsEqualsForm(t *testing.T) {
	options, err := Parse([]string{"--list-test-targets=HEAD~1"})
	if err != nil {
		t.Fatal(err)
	}
	if options.ListTestTargets != "HEAD~1" {
		t.Fatalf("ListTestTargets = %q, want HEAD~1", options.ListTestTargets)
	}
}

func TestParseRejectsListTestTargetsWithOtherScope(t *testing.T) {
	for _, args := range [][]string{
		{"--list-test-targets", "origin/main", "service.go"},
		{"--list-test-targets", "origin/main", "--diff", "HEAD"},
		{"--list-test-targets", "origin/main", "--dry-run"},
		{"--list-test-targets", "origin/main", "--list"},
	} {
		if _, err := Parse(args); err == nil {
			t.Errorf("Parse(%v) succeeded, want an error", args)
		}
	}
}

func TestParseRejectsListTestTargetsWithoutReference(t *testing.T) {
	if _, err := Parse([]string{"--list-test-targets"}); err == nil {
		t.Fatal("Parse() succeeded, want an error")
	}
}
