package result

import (
	"strings"
	"testing"
)

func TestParseStryker(t *testing.T) {
	report := `{
  "schemaVersion": "2.0",
  "files": {
    "src/math.ts": {
      "language": "typescript",
      "source": "export const add = (a, b) => a + b;",
      "mutants": [
        {"id":"1","mutatorName":"ArithmeticOperator","replacement":"a - b","status":"Killed","location":{"start":{"line":1,"column":30},"end":{"line":1,"column":35}}},
        {"id":"2","mutatorName":"BooleanLiteral","status":"NoCoverage","statusReason":"no tests","location":{"start":{"line":2,"column":1},"end":{"line":2,"column":5}}},
        {"id":"3","mutatorName":"StringLiteral","status":"CompileError","location":{"start":{"line":3,"column":1},"end":{"line":3,"column":5}}}
      ]
    }
  }
}`
	run, err := ParseStryker(strings.NewReader(report), Scope{DiffBase: "origin/main"})
	if err != nil {
		t.Fatal(err)
	}
	if run.Backend.ID != "stryker-js" || run.Summary.Total != 3 || run.Summary.Killed != 1 || run.Summary.Uncovered != 1 || run.Summary.Unviable != 1 {
		t.Fatalf("run = %+v", run)
	}
	if got := run.Mutants[0].Location; got.File != "src/math.ts" || got.Line != 1 || got.Column != 30 {
		t.Errorf("location = %+v", got)
	}
}

func TestParseStrykerRejectsMissingFiles(t *testing.T) {
	if _, err := ParseStryker(strings.NewReader(`{"schemaVersion":"2.0"}`), Scope{}); err == nil {
		t.Fatal("ParseStryker() succeeded without files")
	}
}
