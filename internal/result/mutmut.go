package result

import (
	"encoding/json"
	"fmt"
	"io"
)

type mutmutStats struct {
	Killed                    int `json:"killed"`
	Survived                  int `json:"survived"`
	Total                     int `json:"total"`
	NoTests                   int `json:"no_tests"`
	Skipped                   int `json:"skipped"`
	Suspicious                int `json:"suspicious"`
	Timeout                   int `json:"timeout"`
	CheckWasInterruptedByUser int `json:"check_was_interrupted_by_user"`
	Segfault                  int `json:"segfault"`
}

func ParseMutmutStats(reader io.Reader, scope Scope) (Run, error) {
	contents, err := io.ReadAll(reader)
	if err != nil {
		return Run{}, fmt.Errorf("read mutmut CI stats: %w", err)
	}
	var stats mutmutStats
	if err := json.Unmarshal(contents, &stats); err != nil {
		return Run{}, fmt.Errorf("decode mutmut CI stats: %w", err)
	}
	run := Run{
		Backend: Backend{ID: "mutmut"}, Scope: scope, State: StateComplete,
		Summary: Summary{
			Total: stats.Total, Killed: stats.Killed, Survived: stats.Survived,
			Uncovered: stats.NoTests, Skipped: stats.Skipped, TimedOut: stats.Timeout,
			Errors: stats.Suspicious + stats.CheckWasInterruptedByUser + stats.Segfault,
		},
		Raw: append(json.RawMessage(nil), contents...),
	}
	accounted := run.Summary.Killed + run.Summary.Survived + run.Summary.Uncovered + run.Summary.Skipped + run.Summary.TimedOut + run.Summary.Errors
	if stats.Total > accounted {
		run.Summary.Unknown = stats.Total - accounted
	}
	if stats.Total == 0 {
		run.State = StateNoCandidates
	}
	return run, nil
}
