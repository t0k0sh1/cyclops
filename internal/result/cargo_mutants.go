package result

import (
	"encoding/json"
	"fmt"
	"io"
)

type cargoMutant struct {
	Name        string `json:"name"`
	File        string `json:"file"`
	Genre       string `json:"genre"`
	Replacement string `json:"replacement"`
	Span        struct {
		Start struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"start"`
	} `json:"span"`
}

type cargoOutcomes struct {
	Outcomes            []cargoOutcome `json:"outcomes"`
	CargoMutantsVersion string         `json:"cargo_mutants_version"`
}

type cargoOutcome struct {
	Scenario json.RawMessage `json:"scenario"`
	Summary  string          `json:"summary"`
}

type cargoMutantScenario struct {
	Mutant json.RawMessage `json:"Mutant"`
}

func ParseCargoMutants(reader io.Reader, scope Scope) (Run, error) {
	contents, err := io.ReadAll(reader)
	if err != nil {
		return Run{}, fmt.Errorf("read cargo-mutants candidate result: %w", err)
	}
	var mutants []json.RawMessage
	if err := json.Unmarshal(contents, &mutants); err != nil {
		return Run{}, fmt.Errorf("decode cargo-mutants candidate result: %w", err)
	}
	run := Run{
		Backend: Backend{ID: "cargo-mutants"},
		Scope:   scope,
		State:   StateDryRun,
		DryRun:  true,
		Raw:     append(json.RawMessage(nil), contents...),
	}
	for _, raw := range mutants {
		mutant, err := decodeCargoMutant(raw, StatusRunnable)
		if err != nil {
			return Run{}, err
		}
		run.Mutants = append(run.Mutants, mutant)
	}
	run.Recount()
	return run, nil
}

func ParseCargoOutcomes(reader io.Reader, scope Scope) (Run, error) {
	contents, err := io.ReadAll(reader)
	if err != nil {
		return Run{}, fmt.Errorf("read cargo-mutants outcomes: %w", err)
	}
	var output cargoOutcomes
	if err := json.Unmarshal(contents, &output); err != nil {
		return Run{}, fmt.Errorf("decode cargo-mutants outcomes: %w", err)
	}
	run := Run{
		Backend: Backend{ID: "cargo-mutants", Version: output.CargoMutantsVersion},
		Scope:   scope,
		State:   StateComplete,
		Raw:     append(json.RawMessage(nil), contents...),
	}
	for _, outcome := range output.Outcomes {
		var baseline string
		if json.Unmarshal(outcome.Scenario, &baseline) == nil {
			continue
		}
		var scenario cargoMutantScenario
		if err := json.Unmarshal(outcome.Scenario, &scenario); err != nil {
			return Run{}, fmt.Errorf("decode cargo-mutants scenario: %w", err)
		}
		if len(scenario.Mutant) == 0 {
			return Run{}, fmt.Errorf("decode cargo-mutants scenario: missing Mutant")
		}
		mutant, err := decodeCargoMutant(scenario.Mutant, cargoOutcomeStatus(outcome.Summary))
		if err != nil {
			return Run{}, err
		}
		mutant.Raw = append(json.RawMessage(nil), outcome.Scenario...)
		run.Mutants = append(run.Mutants, mutant)
	}
	run.Recount()
	return run, nil
}

func decodeCargoMutant(raw json.RawMessage, status Status) (Mutant, error) {
	var mutation cargoMutant
	if err := json.Unmarshal(raw, &mutation); err != nil {
		return Mutant{}, fmt.Errorf("decode cargo-mutants mutation: %w", err)
	}
	return Mutant{
		Status:      status,
		Location:    Location{File: mutation.File, Line: mutation.Span.Start.Line, Column: mutation.Span.Start.Column},
		Operator:    mutation.Genre,
		Description: mutation.Name,
		Raw:         append(json.RawMessage(nil), raw...),
	}, nil
}

func cargoOutcomeStatus(summary string) Status {
	switch summary {
	case "CaughtMutant":
		return StatusKilled
	case "MissedMutant":
		return StatusSurvived
	case "Timeout":
		return StatusTimedOut
	case "Unviable":
		return StatusUnviable
	default:
		return StatusUnknown
	}
}
