package result

import (
	"strings"
	"testing"
)

func TestParsePIT(t *testing.T) {
	report := `<mutations>
<mutation detected="true" status="KILLED"><sourceFile>Math.java</sourceFile><mutatedClass>com.example.Math</mutatedClass><lineNumber>12</lineNumber><mutator>ConditionalsBoundaryMutator</mutator><description>changed conditional boundary</description></mutation>
<mutation detected="false" status="NO_COVERAGE"><sourceFile>Math.java</sourceFile><mutatedClass>com.example.Math</mutatedClass><lineNumber>20</lineNumber><mutator>VoidMethodCallMutator</mutator></mutation>
</mutations>`
	run, err := ParsePIT(strings.NewReader(report), Scope{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if run.Summary.Total != 2 || run.Summary.Killed != 1 || run.Summary.Uncovered != 1 {
		t.Fatalf("run = %+v", run)
	}
}
