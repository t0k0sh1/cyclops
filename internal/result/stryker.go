package result

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type strykerReport struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Files         map[string]strykerFile `json:"files"`
}

type strykerFile struct {
	Mutants []json.RawMessage `json:"mutants"`
}

type strykerMutant struct {
	Status       string `json:"status"`
	MutatorName  string `json:"mutatorName"`
	Description  string `json:"description"`
	Replacement  string `json:"replacement"`
	StatusReason string `json:"statusReason"`
	Location     struct {
		Start struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"start"`
	} `json:"location"`
}

func ParseStryker(reader io.Reader, scope Scope) (Run, error) {
	contents, err := io.ReadAll(reader)
	if err != nil {
		return Run{}, fmt.Errorf("read StrykerJS report: %w", err)
	}
	var report strykerReport
	if err := json.Unmarshal(contents, &report); err != nil {
		return Run{}, fmt.Errorf("decode StrykerJS report: %w", err)
	}
	if report.Files == nil {
		return Run{}, fmt.Errorf("decode StrykerJS report: missing files")
	}
	run := Run{
		Backend: Backend{ID: "stryker-js"},
		Scope:   scope,
		State:   StateComplete,
		Raw:     append(json.RawMessage(nil), contents...),
	}
	files := make([]string, 0, len(report.Files))
	for file := range report.Files {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		entry := report.Files[file]
		for _, raw := range entry.Mutants {
			var mutation strykerMutant
			if err := json.Unmarshal(raw, &mutation); err != nil {
				return Run{}, fmt.Errorf("decode StrykerJS mutant in %s: %w", file, err)
			}
			description := mutation.Description
			if description == "" {
				description = mutation.Replacement
			}
			if mutation.StatusReason != "" {
				if description != "" {
					description += ": "
				}
				description += mutation.StatusReason
			}
			run.Mutants = append(run.Mutants, Mutant{
				Status:      strykerStatus(mutation.Status),
				Location:    Location{File: file, Line: mutation.Location.Start.Line, Column: mutation.Location.Start.Column},
				Operator:    mutation.MutatorName,
				Description: description,
				Raw:         append(json.RawMessage(nil), raw...),
			})
		}
	}
	run.Recount()
	return run, nil
}

func strykerStatus(status string) Status {
	switch status {
	case "Killed":
		return StatusKilled
	case "Survived":
		return StatusSurvived
	case "NoCoverage":
		return StatusUncovered
	case "Timeout":
		return StatusTimedOut
	case "CompileError", "RuntimeError":
		return StatusUnviable
	case "Ignored":
		return StatusSkipped
	case "Pending":
		return StatusRunnable
	default:
		return StatusUnknown
	}
}
