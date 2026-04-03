package replay

import (
	"github.com/cli-replay/cli-replay/pkg/scenario"
)

// state tracks scenario progress in memory. It mirrors the fields from
// internal/runner.State that are needed for matching, but carries no
// file-system or serialization concerns.
type state struct {
	currentStep int
	totalSteps  int
	stepCounts  []int
	activeGroup *int
	captures    map[string]string
}

func newState(totalSteps int) *state {
	return &state{
		currentStep: 0,
		totalSteps:  totalSteps,
		stepCounts:  make([]int, totalSteps),
		captures:    make(map[string]string),
	}
}

func (s *state) isComplete() bool {
	return s.currentStep >= s.totalSteps
}

func (s *state) enterGroup(idx int) {
	s.activeGroup = &idx
}

func (s *state) exitGroup() {
	s.activeGroup = nil
}

func (s *state) incrementStep(idx int) {
	if idx >= 0 && idx < len(s.stepCounts) {
		s.stepCounts[idx]++
	}
}

func (s *state) stepBudgetRemaining(idx, maxCalls int) int {
	if idx < 0 || idx >= len(s.stepCounts) {
		return 0
	}
	remaining := maxCalls - s.stepCounts[idx]
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *state) groupAllMaxesHit(gr scenario.GroupRange, steps []scenario.Step) bool {
	for i := gr.Start; i < gr.End; i++ {
		if i >= len(steps) || i >= len(s.stepCounts) {
			return false
		}
		bounds := steps[i].EffectiveCalls()
		if s.stepCounts[i] < bounds.Max {
			return false
		}
	}
	return true
}

func (s *state) groupAllMinsMet(gr scenario.GroupRange, steps []scenario.Step) bool {
	for i := gr.Start; i < gr.End; i++ {
		if i >= len(steps) || i >= len(s.stepCounts) {
			return false
		}
		bounds := steps[i].EffectiveCalls()
		if s.stepCounts[i] < bounds.Min {
			return false
		}
	}
	return true
}

// snapshotCaptures returns a copy of the captures map.
func (s *state) snapshotCaptures() map[string]string {
	out := make(map[string]string, len(s.captures))
	for k, v := range s.captures {
		out[k] = v
	}
	return out
}
