package result

import (
	"strings"
	"testing"
)

func TestParseGremlins(t *testing.T) {
	input := `{
  "go_module":"example.com/test",
  "files":[{"file_name":"math.go","mutations":[
    {"type":"ARITHMETIC_BASE","status":"KILLED","line":4,"column":11},
    {"type":"CONDITIONALS_NEGATION","status":"LIVED","line":8,"column":3},
    {"type":"INVERT_LOGICAL","status":"NOT COVERED","line":12,"column":5},
    {"type":"INVERT_NEGATIVES","status":"RUNNABLE","line":16,"column":2},
    {"type":"INVERT_ASSIGNMENTS","status":"SKIPPED","line":17,"column":2},
    {"type":"INVERT_BITWISE","status":"TIMED OUT","line":18,"column":2},
    {"type":"INVERT_LOOPCTRL","status":"NOT VIABLE","line":19,"column":2},
    {"type":"FUTURE","status":"FUTURE STATUS","line":20,"column":1}
  ]}]
}`
	run, err := ParseGremlins(strings.NewReader(input), Scope{DiffBase: "HEAD~1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != StateDryRun || !run.DryRun {
		t.Errorf("run state = %q, dry run = %v", run.State, run.DryRun)
	}
	if got, want := run.Summary, (Summary{Total: 8, Killed: 1, Survived: 1, Uncovered: 1, Skipped: 1, TimedOut: 1, Unviable: 1, Runnable: 1, Unknown: 1}); got != want {
		t.Errorf("summary = %+v, want %+v", got, want)
	}
	if got := run.Mutants[0]; got.Location.File != "math.go" || got.Operator != "ARITHMETIC_BASE" {
		t.Errorf("first mutant = %+v", got)
	}
	if len(run.Mutants[0].Raw) == 0 || len(run.Raw) == 0 {
		t.Error("backend-specific data was not preserved")
	}
}

func TestParseCargoOutcomes(t *testing.T) {
	input := `{
  "cargo_mutants_version":"27.1.0",
  "outcomes":[
    {"scenario":"Baseline","summary":"Success"},
    {"scenario":{"Mutant":{"name":"src/lib.rs:2:5: replace add with 0","file":"src/lib.rs","genre":"FnValue","replacement":"0","span":{"start":{"line":2,"column":5}}}},"summary":"CaughtMutant"},
    {"scenario":{"Mutant":{"name":"src/lib.rs:4:3: replace true with false","file":"src/lib.rs","genre":"BoolValue","replacement":"false","span":{"start":{"line":4,"column":3}}}},"summary":"MissedMutant"},
    {"scenario":{"Mutant":{"name":"src/lib.rs:8:1: timeout","file":"src/lib.rs","genre":"FnValue","replacement":"0","span":{"start":{"line":8,"column":1}}}},"summary":"Timeout"},
    {"scenario":{"Mutant":{"name":"src/lib.rs:12:1: unviable","file":"src/lib.rs","genre":"BinaryOperator","replacement":"-","span":{"start":{"line":12,"column":1}}}},"summary":"Unviable"}
  ]
}`
	run, err := ParseCargoOutcomes(strings.NewReader(input), Scope{Targets: []string{"src/lib.rs"}})
	if err != nil {
		t.Fatal(err)
	}
	if run.Backend.Version != "27.1.0" {
		t.Errorf("backend version = %q", run.Backend.Version)
	}
	if got, want := run.Summary, (Summary{Total: 4, Killed: 1, Survived: 1, TimedOut: 1, Unviable: 1}); got != want {
		t.Errorf("summary = %+v, want %+v", got, want)
	}
	if got := run.Mutants[1]; got.Location.Line != 4 || got.Status != StatusSurvived {
		t.Errorf("surviving mutant = %+v", got)
	}
}

func TestParseCargoMutantsDryRunAndNoCandidates(t *testing.T) {
	input := `[{"name":"src/lib.rs:2:5: replace add with 0","file":"src/lib.rs","genre":"FnValue","replacement":"0","span":{"start":{"line":2,"column":5}}}]`
	run, err := ParseCargoMutants(strings.NewReader(input), Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if run.State != StateDryRun || run.Summary.Runnable != 1 {
		t.Errorf("dry run = %+v", run)
	}

	empty, err := ParseCargoMutants(strings.NewReader(`[]`), Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.State != StateNoCandidates {
		t.Errorf("empty state = %q, want %q", empty.State, StateNoCandidates)
	}
}

func TestParsersRejectMalformedJSON(t *testing.T) {
	if _, err := ParseGremlins(strings.NewReader(`{`), Scope{}, false); err == nil {
		t.Error("ParseGremlins accepted malformed JSON")
	}
	if _, err := ParseCargoOutcomes(strings.NewReader(`{`), Scope{}); err == nil {
		t.Error("ParseCargoOutcomes accepted malformed JSON")
	}
}
