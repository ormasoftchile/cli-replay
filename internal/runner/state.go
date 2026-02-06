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
)

// State tracks scenario progress across CLI invocations.
type State struct {
	ScenarioPath string    `json:"scenario_path"`
	ScenarioHash string    `json:"scenario_hash"`
	CurrentStep  int       `json:"current_step"`
	TotalSteps   int       `json:"total_steps"`
	LastUpdated  time.Time `json:"last_updated"`
}

// Advance increments the current step counter.
func (s *State) Advance() {
	s.CurrentStep++
	s.LastUpdated = time.Now().UTC()
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

// StateFilePath returns the path to the state file for a given scenario path.
// The state file is stored in the system temp directory with a hash of the
// scenario path to ensure uniqueness.
func StateFilePath(scenarioPath string) string {
	hash := sha256.Sum256([]byte(scenarioPath))
	hashStr := hex.EncodeToString(hash[:])[:16]
	return filepath.Join(os.TempDir(), fmt.Sprintf("cli-replay-%s.state", hashStr))
}

// ReadState loads the state from the given file path.
// Returns os.ErrNotExist if the file doesn't exist.
func ReadState(path string) (*State, error) {
	data, err := os.ReadFile(path) //nolint:gosec // State file path is derived, not user input
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
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
		LastUpdated:  time.Now().UTC(),
	}
}
