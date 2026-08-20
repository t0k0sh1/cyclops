package result

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type pitMutations struct {
	Mutations []pitMutation `xml:"mutation"`
}

type pitMutation struct {
	Status       string `xml:"status,attr"`
	SourceFile   string `xml:"sourceFile"`
	MutatedClass string `xml:"mutatedClass"`
	Line         int    `xml:"lineNumber"`
	Mutator      string `xml:"mutator"`
	Description  string `xml:"description"`
}

func ParsePIT(reader io.Reader, scope Scope, dryRun bool) (Run, error) {
	contents, err := io.ReadAll(reader)
	if err != nil {
		return Run{}, fmt.Errorf("read PIT report: %w", err)
	}
	var report pitMutations
	if err := xml.Unmarshal(contents, &report); err != nil {
		return Run{}, fmt.Errorf("decode PIT report: %w", err)
	}
	rawReport, _ := json.Marshal(string(contents))
	run := Run{Backend: Backend{ID: "pit"}, Scope: scope, State: StateComplete, DryRun: dryRun, Raw: rawReport}
	if dryRun {
		run.State = StateDryRun
	}
	for _, mutation := range report.Mutations {
		file := mutation.SourceFile
		if dot := strings.LastIndex(mutation.MutatedClass, "."); dot >= 0 {
			file = strings.ReplaceAll(mutation.MutatedClass[:dot], ".", "/") + "/" + mutation.SourceFile
		}
		mutationRaw, _ := json.Marshal(mutation)
		status := pitStatus(mutation.Status)
		if dryRun && (status == StatusUnknown || mutation.Status == "NOT_STARTED") {
			status = StatusRunnable
		}
		run.Mutants = append(run.Mutants, Mutant{
			Status: status, Location: Location{File: file, Line: mutation.Line},
			Operator: mutation.Mutator, Description: mutation.Description, Raw: mutationRaw,
		})
	}
	run.Recount()
	return run, nil
}

func pitStatus(status string) Status {
	switch status {
	case "KILLED":
		return StatusKilled
	case "SURVIVED":
		return StatusSurvived
	case "NO_COVERAGE":
		return StatusUncovered
	case "TIMED_OUT":
		return StatusTimedOut
	case "NON_VIABLE", "RUN_ERROR", "MEMORY_ERROR":
		return StatusUnviable
	case "NOT_STARTED":
		return StatusRunnable
	default:
		return StatusUnknown
	}
}
