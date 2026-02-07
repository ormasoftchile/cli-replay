// Package verify provides structured output types and formatters for
// cli-replay verification results (JSON, JUnit XML).
package verify

import (
	"strings"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
)

// VerifyResult represents the structured output of a verification run.
type VerifyResult struct {
	Scenario      string       `json:"scenario"`
	Session       string       `json:"session"`
	Passed        bool         `json:"passed"`
	TotalSteps    int          `json:"total_steps"`
	ConsumedSteps int          `json:"consumed_steps"`
	Error         string       `json:"error,omitempty"`
	Steps         []StepResult `json:"steps"`
}

// StepResult represents the verification status of a single step.
type StepResult struct {
	Index     int    `json:"index"`
	Label     string `json:"label"`
	Group     string `json:"group,omitempty"`
	CallCount int    `json:"call_count"`
	Min       int    `json:"min"`
	Max       int    `json:"max"`
	Passed    bool   `json:"passed"`
}

// BuildResult constructs a VerifyResult from a scenario's steps and the
// replay state. The steps parameter should be the flat list of leaf steps
// (from Scenario.FlatSteps()). groupRanges may be nil for scenarios without
// groups. If state is nil, an error result is returned with "no state found".
func BuildResult(scenarioName, session string, steps []scenario.Step, state *runner.State, groupRanges []scenario.GroupRange) *VerifyResult {
	if state == nil {
		return BuildErrorResult(scenarioName, session, "no state found")
	}

	// Build lookup from flat index → group name
	groupNameByIndex := make(map[int]string)
	for _, gr := range groupRanges {
		for i := gr.Start; i < gr.End; i++ {
			groupNameByIndex[i] = gr.Name
		}
	}

	result := &VerifyResult{
		Scenario: scenarioName,
		Session:  session,
		Steps:    make([]StepResult, len(steps)),
	}

	consumed := 0
	allPassed := true

	for i, step := range steps {
		bounds := step.EffectiveCalls()
		callCount := 0
		if state.StepCounts != nil && i < len(state.StepCounts) {
			callCount = state.StepCounts[i]
		}

		passed := callCount >= bounds.Min
		if !passed {
			allPassed = false
		}
		if callCount >= 1 {
			consumed++
		}

		label := StepLabel(step)
		groupName := groupNameByIndex[i]
		if groupName != "" {
			label = "[group:" + groupName + "] " + label
		}

		result.Steps[i] = StepResult{
			Index:     i,
			Label:     label,
			Group:     groupName,
			CallCount: callCount,
			Min:       bounds.Min,
			Max:       bounds.Max,
			Passed:    passed,
		}
	}

	result.TotalSteps = len(steps)
	result.ConsumedSteps = consumed
	result.Passed = allPassed

	return result
}

// BuildErrorResult constructs a VerifyResult representing an error condition
// (e.g., no state file found).
func BuildErrorResult(scenarioName, session, errMsg string) *VerifyResult {
	return &VerifyResult{
		Scenario: scenarioName,
		Session:  session,
		Passed:   false,
		Error:    errMsg,
		Steps:    []StepResult{},
	}
}

// StepLabel builds a human-readable label from a step's argv.
func StepLabel(step scenario.Step) string {
	return strings.Join(step.Match.Argv, " ")
}
