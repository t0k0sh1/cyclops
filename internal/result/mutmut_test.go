package result

import (
	"strings"
	"testing"
)

func TestParseMutmutStats(t *testing.T) {
	input := `{"killed":4,"survived":2,"total":10,"no_tests":1,"skipped":1,"suspicious":0,"timeout":1,"check_was_interrupted_by_user":0,"segfault":0}`
	run, err := ParseMutmutStats(strings.NewReader(input), Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if run.Summary.Killed != 4 || run.Summary.Survived != 2 || run.Summary.Uncovered != 1 || run.Summary.TimedOut != 1 || run.Summary.Unknown != 1 {
		t.Fatalf("summary = %+v", run.Summary)
	}
}
