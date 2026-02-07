// Package runner provides the core replay logic including state management.
package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cli-replay/cli-replay/internal/scenario"
)

// State tracks scenario progress across CLI invocations.
type State struct {
	ScenarioPath  string    `json:"scenario_path"`
	ScenarioHash  string    `json:"scenario_hash"`
	CurrentStep   int       `json:"current_step"`
	TotalSteps    int       `json:"total_steps"`
	StepCounts    []int     `json:"step_counts,omitempty"`
	ConsumedSteps []bool    `json:"consumed_steps,omitempty"` // deprecated: read-only migration
	ActiveGroup   *int      `json:"active_group,omitempty"`
	InterceptDir  string    `json:"intercept_dir,omitempty"`
	LastUpdated   time.Time `json:"last_updated"`
}

// IsInGroup returns true if the state is currently inside a step group.
func (s *State) IsInGroup() bool {
	return s.ActiveGroup != nil
}

// CurrentGroupRange returns the GroupRange for the currently active group.
// Returns nil if the state is not inside a group.
func (s *State) CurrentGroupRange(ranges []scenario.GroupRange) *scenario.GroupRange {
	if s.ActiveGroup == nil {
		return nil
	}
	idx := *s.ActiveGroup
	if idx < 0 || idx >= len(ranges) {
		return nil
	}
	return &ranges[idx]
}

// EnterGroup sets the active group index.
func (s *State) EnterGroup(groupIdx int) {
	s.ActiveGroup = &groupIdx
	s.LastUpdated = time.Now().UTC()
}

// ExitGroup clears the active group, indicating the state has left a group.
func (s *State) ExitGroup() {
	s.ActiveGroup = nil
	s.LastUpdated = time.Now().UTC()
}

// FindGroupContaining returns the index into groupRanges that contains the
// given flat step index, or -1 if the index is not inside any group.
func FindGroupContaining(ranges []scenario.GroupRange, flatIdx int) int {
	for i, gr := range ranges {
		if flatIdx >= gr.Start && flatIdx < gr.End {
			return i
		}
	}
	return -1
}

// GroupAllMaxesHit returns true if every step in the given group range has
// reached its maximum call count.
func (s *State) GroupAllMaxesHit(gr scenario.GroupRange, steps []scenario.Step) bool {
	for i := gr.Start; i < gr.End; i++ {
		if i >= len(steps) || i >= len(s.StepCounts) {
			return false
		}
		bounds := steps[i].EffectiveCalls()
		if s.StepCounts[i] < bounds.Max {
			return false
		}
	}
	return true
}

// GroupAllMinsmet returns true if every step in the given group range has
// met its minimum call count.
func (s *State) GroupAllMinsMet(gr scenario.GroupRange, steps []scenario.Step) bool {
	for i := gr.Start; i < gr.End; i++ {
		if i >= len(steps) || i >= len(s.StepCounts) {
			return false
		}
		bounds := steps[i].EffectiveCalls()
		if s.StepCounts[i] < bounds.Min {
			return false
		}
	}
	return true
}

// Advance increments the current step counter and marks the step as consumed.
func (s *State) Advance() {
	if s.StepCounts != nil && s.CurrentStep < len(s.StepCounts) {
		s.StepCounts[s.CurrentStep]++
	}
	s.CurrentStep++
	s.LastUpdated = time.Now().UTC()
}

// AdvanceStep marks a specific step index as consumed (for out-of-order consumption).
func (s *State) AdvanceStep(idx int) {
	if s.StepCounts == nil {
		s.StepCounts = make([]int, s.TotalSteps)
	}
	if idx >= 0 && idx < len(s.StepCounts) {
		s.StepCounts[idx]++
	}
	s.LastUpdated = time.Now().UTC()
}

// AllStepsConsumed returns true if every step has been invoked at least once.
func (s *State) AllStepsConsumed() bool {
	if s.StepCounts == nil {
		return s.CurrentStep >= s.TotalSteps
	}
	for _, c := range s.StepCounts {
		if c < 1 {
			return false
		}
	}
	return true
}

// IsStepConsumed returns true if the step at the given index has been invoked at least once.
func (s *State) IsStepConsumed(idx int) bool {
	if s.StepCounts == nil {
		return idx < s.CurrentStep
	}
	if idx < 0 || idx >= len(s.StepCounts) {
		return false
	}
	return s.StepCounts[idx] >= 1
}

// IncrementStep increments the invocation count for a specific step index
// without advancing CurrentStep. Used by the call-count-bounds loop when
// re-matching the same step.
func (s *State) IncrementStep(idx int) {
	if s.StepCounts == nil {
		s.StepCounts = make([]int, s.TotalSteps)
	}
	if idx >= 0 && idx < len(s.StepCounts) {
		s.StepCounts[idx]++
	}
	s.LastUpdated = time.Now().UTC()
}

// StepBudgetRemaining returns how many more calls step[idx] can accept
// given a maximum call count. Returns 0 when the budget is exhausted.
func (s *State) StepBudgetRemaining(idx, maxCalls int) int {
	if s.StepCounts == nil || idx < 0 || idx >= len(s.StepCounts) {
		return 0
	}
	remaining := maxCalls - s.StepCounts[idx]
	if remaining < 0 {
		return 0
	}
	return remaining
}

// AllStepsMetMin returns true if every step has been invoked at least its
// minimum required number of times. Steps without explicit CallBounds
// default to min=1 via EffectiveCalls().
func (s *State) AllStepsMetMin(steps []scenario.Step) bool {
	if s.StepCounts == nil {
		return false
	}
	for i, step := range steps {
		bounds := step.EffectiveCalls()
		if i >= len(s.StepCounts) {
			if bounds.Min > 0 {
				return false
			}
			continue
		}
		if s.StepCounts[i] < bounds.Min {
			return false
		}
	}
	return true
}

// IsComplete returns true if all steps have been consumed.
func (s *State) IsComplete() bool {
	return s.CurrentStep >= s.TotalSteps
}

// RemainingSteps returns the number of steps not yet consumed.
func (s *State) RemainingSteps() int {
	remaining := s.TotalSteps - s.CurrentStep
	if remaining < 0 {
		return 0
	}
	return remaining
}

// cliReplayDir returns the .cli-replay/ directory for a given scenario path.
// The directory is created adjacent to the scenario file.
func cliReplayDir(scenarioPath string) string {
	return filepath.Join(filepath.Dir(scenarioPath), ".cli-replay")
}

// StateFilePath returns the path to the state file for a given scenario path.
// The state file is stored in .cli-replay/ next to the scenario file, with a
// hash of the scenario path to ensure uniqueness.
// If CLI_REPLAY_SESSION is set, it is included in the hash to allow parallel sessions.
func StateFilePath(scenarioPath string) string {
	return StateFilePathWithSession(scenarioPath, os.Getenv("CLI_REPLAY_SESSION"))
}

// StateFilePathWithSession returns the state file path for a given scenario
// and session ID. When session is non-empty, each session gets its own state
// file, enabling parallel test execution against the same scenario.
func StateFilePathWithSession(scenarioPath, session string) string {
	key := scenarioPath
	if session != "" {
		key = scenarioPath + "\x00" + session
	}
	hash := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(hash[:])[:16]
	dir := cliReplayDir(scenarioPath)
	return filepath.Join(dir, fmt.Sprintf("cli-replay-%s.state", hashStr))
}

// InterceptDirPath creates an intercept directory inside .cli-replay/ next to
// the scenario file. Returns the path to the created directory.
func InterceptDirPath(scenarioPath string) (string, error) {
	dir := cliReplayDir(scenarioPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("failed to create .cli-replay directory: %w", err)
	}
	return os.MkdirTemp(dir, "intercept-")
}

// ReadState loads the state from the given file path.
// Returns os.ErrNotExist if the file doesn't exist.
// Migrates legacy ConsumedSteps []bool to StepCounts []int if needed.
func ReadState(path string) (*State, error) {
	data, err := os.ReadFile(path) //nolint:gosec // State file path is derived, not user input
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Migration: convert legacy ConsumedSteps to StepCounts
	if state.StepCounts == nil && state.ConsumedSteps != nil {
		state.StepCounts = make([]int, len(state.ConsumedSteps))
		for i, consumed := range state.ConsumedSteps {
			if consumed {
				state.StepCounts[i] = 1
			}
		}
		state.ConsumedSteps = nil
	}

	return &state, nil
}

// WriteState persists the state to the given file path.
// Uses atomic write (write to temp file, then rename) to prevent corruption.
func WriteState(path string, state *State) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Marshal state to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file first
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, path); err != nil {
		// Clean up temp file on failure
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// DeleteState removes the state file at the given path.
// Does not return an error if the file doesn't exist.
func DeleteState(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete state file: %w", err)
	}
	return nil
}

// NewState creates a new state for the given scenario.
func NewState(scenarioPath, scenarioHash string, totalSteps int) *State {
	return &State{
		ScenarioPath: scenarioPath,
		ScenarioHash: scenarioHash,
		CurrentStep:  0,
		TotalSteps:   totalSteps,
		StepCounts:   make([]int, totalSteps),
		LastUpdated:  time.Now().UTC(),
	}
}
