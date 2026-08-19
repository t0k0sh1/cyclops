package result

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type gremlinsOutput struct {
	Files []struct {
		Filename  string            `json:"file_name"`
		Mutations []json.RawMessage `json:"mutations"`
	} `json:"files"`
}

type gremlinsMutation struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

func ParseGremlins(reader io.Reader, scope Scope, dryRun bool) (Run, error) {
	contents, err := io.ReadAll(reader)
	if err != nil {
		return Run{}, fmt.Errorf("read Gremlins result: %w", err)
	}
	var output gremlinsOutput
	if err := json.Unmarshal(contents, &output); err != nil {
		return Run{}, fmt.Errorf("decode Gremlins result: %w", err)
	}
	run := Run{
		Backend: Backend{ID: "gremlins"},
		Scope:   scope,
		State:   StateComplete,
		DryRun:  dryRun,
		Raw:     append(json.RawMessage(nil), contents...),
	}
	if dryRun {
		run.State = StateDryRun
	}
	for _, file := range output.Files {
		for _, raw := range file.Mutations {
			var mutation gremlinsMutation
			if err := json.Unmarshal(raw, &mutation); err != nil {
				return Run{}, fmt.Errorf("decode Gremlins mutation in %s: %w", file.Filename, err)
			}
			run.Mutants = append(run.Mutants, Mutant{
				Status:   gremlinsStatus(mutation.Status),
				Location: Location{File: file.Filename, Line: mutation.Line, Column: mutation.Column},
				Operator: mutation.Type,
				Raw:      append(json.RawMessage(nil), raw...),
			})
		}
	}
	run.Recount()
	return run, nil
}

func gremlinsStatus(status string) Status {
	switch strings.ToUpper(status) {
	case "KILLED":
		return StatusKilled
	case "LIVED":
		return StatusSurvived
	case "NOT COVERED":
		return StatusUncovered
	case "SKIPPED":
		return StatusSkipped
	case "TIMED OUT":
		return StatusTimedOut
	case "NOT VIABLE":
		return StatusUnviable
	case "RUNNABLE":
		return StatusRunnable
	default:
		return StatusUnknown
	}
}
