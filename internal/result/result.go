package result

import "encoding/json"

type Status string

const (
	StatusKilled    Status = "killed"
	StatusSurvived  Status = "survived"
	StatusUncovered Status = "uncovered"
	StatusSkipped   Status = "skipped"
	StatusTimedOut  Status = "timed-out"
	StatusUnviable  Status = "unviable"
	StatusRunnable  Status = "runnable"
	StatusError     Status = "error"
	StatusUnknown   Status = "unknown"
)

type State string

const (
	StateComplete     State = "complete"
	StateDryRun       State = "dry-run"
	StateNoCandidates State = "no-candidates"
	StatePartial      State = "partial"
	StateFailed       State = "failed"
	StateSkipped      State = "skipped"
)

type Run struct {
	Backend Backend          `json:"backend"`
	Scope   Scope            `json:"scope"`
	State   State            `json:"state"`
	DryRun  bool             `json:"dry_run"`
	Summary Summary          `json:"summary"`
	Mutants []Mutant         `json:"mutants,omitempty"`
	Errors  []ExecutionError `json:"errors,omitempty"`
	Raw     json.RawMessage  `json:"backend_data,omitempty"`
}

type Backend struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

type Scope struct {
	DiffBase string   `json:"diff_base,omitempty"`
	Head     string   `json:"head,omitempty"`
	Targets  []string `json:"targets,omitempty"`
}

type Summary struct {
	Total     int `json:"total"`
	Killed    int `json:"killed"`
	Survived  int `json:"survived"`
	Uncovered int `json:"uncovered"`
	Skipped   int `json:"skipped"`
	TimedOut  int `json:"timed_out"`
	Unviable  int `json:"unviable"`
	Runnable  int `json:"runnable"`
	Errors    int `json:"errors"`
	Unknown   int `json:"unknown"`
}

type Mutant struct {
	Status      Status          `json:"status"`
	Location    Location        `json:"location"`
	Operator    string          `json:"operator,omitempty"`
	Description string          `json:"description,omitempty"`
	Raw         json.RawMessage `json:"backend_data,omitempty"`
}

type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

type ExecutionError struct {
	Kind     string `json:"kind"`
	Message  string `json:"message"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

func (run *Run) Recount() {
	run.Summary = Summary{Total: len(run.Mutants)}
	for _, mutant := range run.Mutants {
		switch mutant.Status {
		case StatusKilled:
			run.Summary.Killed++
		case StatusSurvived:
			run.Summary.Survived++
		case StatusUncovered:
			run.Summary.Uncovered++
		case StatusSkipped:
			run.Summary.Skipped++
		case StatusTimedOut:
			run.Summary.TimedOut++
		case StatusUnviable:
			run.Summary.Unviable++
		case StatusRunnable:
			run.Summary.Runnable++
		case StatusError:
			run.Summary.Errors++
		default:
			run.Summary.Unknown++
		}
	}
	if len(run.Mutants) == 0 && len(run.Errors) == 0 && run.State != StateSkipped {
		run.State = StateNoCandidates
	}
}
