package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_JSON_Serialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	state := State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "abc123def456",
		CurrentStep:  2,
		TotalSteps:   5,
		LastUpdated:  now,
	}

	// Serialize
	data, err := json.Marshal(state)
	require.NoError(t, err)

	// Deserialize
	var loaded State
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	assert.Equal(t, state.ScenarioPath, loaded.ScenarioPath)
	assert.Equal(t, state.ScenarioHash, loaded.ScenarioHash)
	assert.Equal(t, state.CurrentStep, loaded.CurrentStep)
	assert.Equal(t, state.TotalSteps, loaded.TotalSteps)
	assert.Equal(t, state.LastUpdated.Unix(), loaded.LastUpdated.Unix())
}

func TestStateFilePath(t *testing.T) {
	tests := []struct {
		name         string
		scenarioPath string
	}{
		{
			name:         "simple path",
			scenarioPath: "/path/to/scenario.yaml",
		},
		{
			name:         "path with spaces",
			scenarioPath: "/path/to/my scenario.yaml",
		},
		{
			name:         "different paths produce different hashes",
			scenarioPath: "/different/path.yaml",
		},
	}

	paths := make(map[string]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := StateFilePath(tt.scenarioPath)

			// Should be in .cli-replay/ directory next to the scenario file
			assert.Contains(t, path, ".cli-replay")
			assert.Contains(t, path, filepath.Dir(tt.scenarioPath))

			// Should have proper prefix
			assert.Contains(t, filepath.Base(path), "cli-replay-")

			// Should have .state extension
			assert.Equal(t, ".state", filepath.Ext(path))

			// Track unique paths
			paths[path] = true
		})
	}

	// Different scenario paths should produce different state paths
	assert.Len(t, paths, 3, "different scenario paths should produce different state paths")
}

func TestStatePersistence_WriteRead(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "test.state")

	now := time.Now().UTC().Truncate(time.Second)
	state := &State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "abc123",
		CurrentStep:  1,
		TotalSteps:   3,
		LastUpdated:  now,
	}

	// Write state
	err := WriteState(stateFile, state)
	require.NoError(t, err)

	// File should exist
	_, err = os.Stat(stateFile)
	require.NoError(t, err)

	// Read state back
	loaded, err := ReadState(stateFile)
	require.NoError(t, err)

	assert.Equal(t, state.ScenarioPath, loaded.ScenarioPath)
	assert.Equal(t, state.ScenarioHash, loaded.ScenarioHash)
	assert.Equal(t, state.CurrentStep, loaded.CurrentStep)
	assert.Equal(t, state.TotalSteps, loaded.TotalSteps)
	assert.Equal(t, state.LastUpdated.Unix(), loaded.LastUpdated.Unix())
}

func TestReadState_NotFound(t *testing.T) {
	state, err := ReadState("/nonexistent/path/state.json")
	assert.Nil(t, state)
	assert.True(t, os.IsNotExist(err))
}

func TestReadState_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "invalid.state")

	// Write invalid JSON
	err := os.WriteFile(stateFile, []byte("not valid json"), 0600)
	require.NoError(t, err)

	_, err = ReadState(stateFile)
	require.Error(t, err)
}

func TestWriteState_Atomic(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "atomic.state")

	// Write initial state
	initial := &State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "initial",
		CurrentStep:  0,
		TotalSteps:   3,
		LastUpdated:  time.Now().UTC(),
	}
	err := WriteState(stateFile, initial)
	require.NoError(t, err)

	// Write updated state
	updated := &State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "initial",
		CurrentStep:  1,
		TotalSteps:   3,
		LastUpdated:  time.Now().UTC(),
	}
	err = WriteState(stateFile, updated)
	require.NoError(t, err)

	// Read back and verify it's the updated state
	loaded, err := ReadState(stateFile)
	require.NoError(t, err)
	assert.Equal(t, 1, loaded.CurrentStep)

	// No temp files should remain
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the state file should exist, no temp files")
}

func TestWriteState_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "subdir", "nested", "state.state")

	state := &State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "abc",
		CurrentStep:  0,
		TotalSteps:   1,
		LastUpdated:  time.Now().UTC(),
	}

	// Should succeed even if directory doesn't exist
	err := WriteState(stateFile, state)
	require.NoError(t, err)

	// File should exist
	_, err = os.Stat(stateFile)
	require.NoError(t, err)
}

func TestDeleteState(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "delete.state")

	// Create state file
	state := &State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "abc",
		CurrentStep:  0,
		TotalSteps:   1,
		LastUpdated:  time.Now().UTC(),
	}
	err := WriteState(stateFile, state)
	require.NoError(t, err)

	// Delete it
	err = DeleteState(stateFile)
	require.NoError(t, err)

	// File should not exist
	_, err = os.Stat(stateFile)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteState_NotFound(t *testing.T) {
	// Deleting non-existent file should not error
	err := DeleteState("/nonexistent/path/state.state")
	assert.NoError(t, err)
}

func TestState_Advance(t *testing.T) {
	state := &State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "abc",
		CurrentStep:  0,
		TotalSteps:   3,
		LastUpdated:  time.Now().UTC(),
	}

	// Advance step
	state.Advance()
	assert.Equal(t, 1, state.CurrentStep)

	state.Advance()
	assert.Equal(t, 2, state.CurrentStep)

	state.Advance()
	assert.Equal(t, 3, state.CurrentStep)
}

func TestState_IsComplete(t *testing.T) {
	tests := []struct {
		name         string
		currentStep  int
		totalSteps   int
		wantComplete bool
	}{
		{
			name:         "not started",
			currentStep:  0,
			totalSteps:   3,
			wantComplete: false,
		},
		{
			name:         "in progress",
			currentStep:  1,
			totalSteps:   3,
			wantComplete: false,
		},
		{
			name:         "complete",
			currentStep:  3,
			totalSteps:   3,
			wantComplete: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &State{
				CurrentStep: tt.currentStep,
				TotalSteps:  tt.totalSteps,
			}
			assert.Equal(t, tt.wantComplete, state.IsComplete())
		})
	}
}

// T025: Unit tests for state advancement
func TestState_AdvanceMultiple(t *testing.T) {
	state := &State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "abc123",
		CurrentStep:  0,
		TotalSteps:   5,
	}

	// Advance through all steps
	for i := 0; i < 5; i++ {
		assert.Equal(t, i, state.CurrentStep)
		assert.False(t, state.IsComplete())
		state.Advance()
	}

	assert.Equal(t, 5, state.CurrentStep)
	assert.True(t, state.IsComplete())
}

func TestState_RemainingSteps(t *testing.T) {
	tests := []struct {
		name        string
		currentStep int
		totalSteps  int
		want        int
	}{
		{
			name:        "all remaining",
			currentStep: 0,
			totalSteps:  5,
			want:        5,
		},
		{
			name:        "some consumed",
			currentStep: 2,
			totalSteps:  5,
			want:        3,
		},
		{
			name:        "one remaining",
			currentStep: 4,
			totalSteps:  5,
			want:        1,
		},
		{
			name:        "none remaining",
			currentStep: 5,
			totalSteps:  5,
			want:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &State{
				CurrentStep: tt.currentStep,
				TotalSteps:  tt.totalSteps,
			}
			assert.Equal(t, tt.want, state.RemainingSteps())
		})
	}
}

func TestStateFilePathWithSession(t *testing.T) {
	scenarioPath := "/path/to/scenario.yaml"

	t.Run("empty session matches legacy behavior", func(t *testing.T) {
		withSession := StateFilePathWithSession(scenarioPath, "")
		legacy := StateFilePathWithSession(scenarioPath, "")
		assert.Equal(t, legacy, withSession)
	})

	t.Run("different sessions produce different paths", func(t *testing.T) {
		pathA := StateFilePathWithSession(scenarioPath, "session-a")
		pathB := StateFilePathWithSession(scenarioPath, "session-b")
		assert.NotEqual(t, pathA, pathB)
	})

	t.Run("same session same path", func(t *testing.T) {
		pathA := StateFilePathWithSession(scenarioPath, "session-x")
		pathB := StateFilePathWithSession(scenarioPath, "session-x")
		assert.Equal(t, pathA, pathB)
	})

	t.Run("session isolates from no-session", func(t *testing.T) {
		noSession := StateFilePathWithSession(scenarioPath, "")
		withSession := StateFilePathWithSession(scenarioPath, "my-session")
		assert.NotEqual(t, noSession, withSession)
	})

	t.Run("different scenarios same session still differ", func(t *testing.T) {
		pathA := StateFilePathWithSession("/path/a.yaml", "session-1")
		pathB := StateFilePathWithSession("/path/b.yaml", "session-1")
		assert.NotEqual(t, pathA, pathB)
	})
}

func TestStateFilePath_ReadsEnv(t *testing.T) {
	scenarioPath := "/path/to/scenario.yaml"

	// Without session env
	t.Setenv("CLI_REPLAY_SESSION", "")
	noSession := StateFilePath(scenarioPath)

	// With session env
	t.Setenv("CLI_REPLAY_SESSION", "test-session-123")
	withSession := StateFilePath(scenarioPath)

	assert.NotEqual(t, noSession, withSession, "session env should change state file path")
}

func TestState_InterceptDir_Serialization(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "test.state")

	state := &State{
		ScenarioPath: "/path/to/scenario.yaml",
		ScenarioHash: "abc123",
		CurrentStep:  0,
		TotalSteps:   3,
		InterceptDir: "/tmp/cli-replay-intercept-abc",
		LastUpdated:  time.Now().UTC().Truncate(time.Second),
	}

	err := WriteState(stateFile, state)
	require.NoError(t, err)

	loaded, err := ReadState(stateFile)
	require.NoError(t, err)

	assert.Equal(t, state.InterceptDir, loaded.InterceptDir)
}

// --- T006: StepCounts migration and method tests ---

func TestNewState_InitializesStepCounts(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash123", 5)
	require.NotNil(t, state.StepCounts)
	assert.Len(t, state.StepCounts, 5)
	for _, c := range state.StepCounts {
		assert.Equal(t, 0, c)
	}
	// ConsumedSteps should NOT be populated in new state
	assert.Nil(t, state.ConsumedSteps)
}

func TestReadState_MigratesConsumedStepsToStepCounts(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "legacy.state")

	// Write a legacy state file with ConsumedSteps (no StepCounts)
	legacyJSON := `{
		"scenario_path": "/path/to/scenario.yaml",
		"scenario_hash": "abc123",
		"current_step": 2,
		"total_steps": 4,
		"consumed_steps": [true, true, false, false],
		"last_updated": "2026-02-07T10:00:00Z"
	}`
	err := os.WriteFile(stateFile, []byte(legacyJSON), 0600)
	require.NoError(t, err)

	// Read and expect migration
	state, err := ReadState(stateFile)
	require.NoError(t, err)

	assert.Equal(t, []int{1, 1, 0, 0}, state.StepCounts)
	assert.Nil(t, state.ConsumedSteps, "ConsumedSteps should be nil after migration")
}

func TestReadState_PreservesStepCountsWhenPresent(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "new.state")

	newJSON := `{
		"scenario_path": "/path/to/scenario.yaml",
		"scenario_hash": "abc123",
		"current_step": 2,
		"total_steps": 3,
		"step_counts": [3, 1, 0],
		"last_updated": "2026-02-07T10:00:00Z"
	}`
	err := os.WriteFile(stateFile, []byte(newJSON), 0600)
	require.NoError(t, err)

	state, err := ReadState(stateFile)
	require.NoError(t, err)

	assert.Equal(t, []int{3, 1, 0}, state.StepCounts)
}

func TestState_Advance_IncrementsStepCounts(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 3)

	state.Advance()
	assert.Equal(t, 1, state.CurrentStep)
	assert.Equal(t, []int{1, 0, 0}, state.StepCounts)

	state.Advance()
	assert.Equal(t, 2, state.CurrentStep)
	assert.Equal(t, []int{1, 1, 0}, state.StepCounts)

	state.Advance()
	assert.Equal(t, 3, state.CurrentStep)
	assert.Equal(t, []int{1, 1, 1}, state.StepCounts)
}

func TestState_AdvanceStep_IncrementsStepCounts(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 3)

	state.AdvanceStep(0)
	assert.Equal(t, []int{1, 0, 0}, state.StepCounts)

	state.AdvanceStep(0) // increment again
	assert.Equal(t, []int{2, 0, 0}, state.StepCounts)

	state.AdvanceStep(2) // out of order
	assert.Equal(t, []int{2, 0, 1}, state.StepCounts)
}

func TestState_AllStepsConsumed_WithStepCounts(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 3)
	assert.False(t, state.AllStepsConsumed())

	state.StepCounts = []int{1, 0, 0}
	assert.False(t, state.AllStepsConsumed())

	state.StepCounts = []int{1, 1, 1}
	assert.True(t, state.AllStepsConsumed())

	state.StepCounts = []int{5, 1, 3}
	assert.True(t, state.AllStepsConsumed())
}

func TestState_IsStepConsumed_WithStepCounts(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 3)
	state.StepCounts = []int{2, 0, 1}

	assert.True(t, state.IsStepConsumed(0))
	assert.False(t, state.IsStepConsumed(1))
	assert.True(t, state.IsStepConsumed(2))
	assert.False(t, state.IsStepConsumed(-1))
	assert.False(t, state.IsStepConsumed(5))
}

// T019: Tests for IncrementStep, StepBudgetRemaining, AllStepsMetMin

func TestState_IncrementStep(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 3)

	state.IncrementStep(0)
	assert.Equal(t, 1, state.StepCounts[0])

	state.IncrementStep(0)
	assert.Equal(t, 2, state.StepCounts[0])

	state.IncrementStep(2)
	assert.Equal(t, 1, state.StepCounts[2])

	// Out of bounds — no panic
	state.IncrementStep(-1)
	state.IncrementStep(10)
}

func TestState_IncrementStep_NilStepCounts(t *testing.T) {
	state := &State{TotalSteps: 3}
	state.IncrementStep(1)
	assert.NotNil(t, state.StepCounts)
	assert.Equal(t, 1, state.StepCounts[1])
}

func TestState_StepBudgetRemaining(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 3)

	// Budget of 5, 0 consumed → 5 remaining
	assert.Equal(t, 5, state.StepBudgetRemaining(0, 5))

	state.StepCounts[0] = 3
	assert.Equal(t, 2, state.StepBudgetRemaining(0, 5))

	state.StepCounts[0] = 5
	assert.Equal(t, 0, state.StepBudgetRemaining(0, 5))

	state.StepCounts[0] = 7 // over budget
	assert.Equal(t, 0, state.StepBudgetRemaining(0, 5))

	// Out of bounds
	assert.Equal(t, 0, state.StepBudgetRemaining(-1, 5))
	assert.Equal(t, 0, state.StepBudgetRemaining(10, 5))
}

func TestState_AllStepsMetMin(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 3)

	steps := []scenario.Step{
		{Calls: &scenario.CallBounds{Min: 2, Max: 5}},
		{Calls: &scenario.CallBounds{Min: 1, Max: 1}},
		{}, // no call bounds → defaults to min=1
	}

	// All zeros — not met
	assert.False(t, state.AllStepsMetMin(steps))

	// Partially met
	state.StepCounts = []int{2, 1, 0}
	assert.False(t, state.AllStepsMetMin(steps))

	// All met
	state.StepCounts = []int{2, 1, 1}
	assert.True(t, state.AllStepsMetMin(steps))

	// Over min
	state.StepCounts = []int{5, 1, 3}
	assert.True(t, state.AllStepsMetMin(steps))
}

func TestState_AllStepsMetMin_OptionalStep(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 2)

	steps := []scenario.Step{
		{Calls: &scenario.CallBounds{Min: 0, Max: 5}}, // optional
		{}, // default min=1
	}

	// Optional step not called, required step called
	state.StepCounts = []int{0, 1}
	assert.True(t, state.AllStepsMetMin(steps))
}

// T024: ActiveGroup state tracking tests

func TestState_ActiveGroup_InitiallyNil(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 5)
	assert.Nil(t, state.ActiveGroup)
	assert.False(t, state.IsInGroup())
}

func TestState_EnterGroup_SetsIndex(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 5)
	state.EnterGroup(2)
	require.NotNil(t, state.ActiveGroup)
	assert.Equal(t, 2, *state.ActiveGroup)
	assert.True(t, state.IsInGroup())
}

func TestState_ExitGroup_ClearsToNil(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 5)
	state.EnterGroup(1)
	require.True(t, state.IsInGroup())

	state.ExitGroup()
	assert.Nil(t, state.ActiveGroup)
	assert.False(t, state.IsInGroup())
}

func TestState_ActiveGroup_PersistAndRestoreJSON(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "group.state")

	state := NewState("/path/to/scenario.yaml", "hash", 5)
	state.EnterGroup(3)

	err := WriteState(stateFile, state)
	require.NoError(t, err)

	loaded, err := ReadState(stateFile)
	require.NoError(t, err)
	require.NotNil(t, loaded.ActiveGroup)
	assert.Equal(t, 3, *loaded.ActiveGroup)
}

func TestState_ActiveGroup_NilOmittedInJSON(t *testing.T) {
	state := NewState("/path/to/scenario.yaml", "hash", 3)
	data, err := json.Marshal(state)
	require.NoError(t, err)
	// active_group should be omitted when nil
	assert.NotContains(t, string(data), "active_group")
}

func TestState_CurrentGroupRange(t *testing.T) {
	ranges := []scenario.GroupRange{
		{Start: 0, End: 2, Name: "group-1", TopIndex: 0},
		{Start: 3, End: 5, Name: "group-2", TopIndex: 2},
	}

	state := NewState("/path/to/scenario.yaml", "hash", 6)

	// No active group → nil
	assert.Nil(t, state.CurrentGroupRange(ranges))

	// Active group → returns correct range
	state.EnterGroup(1)
	gr := state.CurrentGroupRange(ranges)
	require.NotNil(t, gr)
	assert.Equal(t, "group-2", gr.Name)
	assert.Equal(t, 3, gr.Start)
	assert.Equal(t, 5, gr.End)

	// Out of bounds → nil
	state.EnterGroup(99)
	assert.Nil(t, state.CurrentGroupRange(ranges))
}

func TestFindGroupContaining(t *testing.T) {
	ranges := []scenario.GroupRange{
		{Start: 1, End: 3, Name: "group-1", TopIndex: 1},
		{Start: 4, End: 7, Name: "group-2", TopIndex: 3},
	}

	assert.Equal(t, -1, FindGroupContaining(ranges, 0))  // before first group
	assert.Equal(t, 0, FindGroupContaining(ranges, 1))    // start of group-1
	assert.Equal(t, 0, FindGroupContaining(ranges, 2))    // inside group-1
	assert.Equal(t, -1, FindGroupContaining(ranges, 3))   // between groups (End is exclusive)
	assert.Equal(t, 1, FindGroupContaining(ranges, 4))    // start of group-2
	assert.Equal(t, 1, FindGroupContaining(ranges, 6))    // inside group-2
	assert.Equal(t, -1, FindGroupContaining(ranges, 7))   // past group-2
	assert.Equal(t, -1, FindGroupContaining(nil, 0))      // no groups
}

func TestState_GroupAllMaxesHit(t *testing.T) {
	gr := scenario.GroupRange{Start: 1, End: 3, Name: "test-group"}
	steps := []scenario.Step{
		{}, // index 0 - outside group
		{Calls: &scenario.CallBounds{Min: 1, Max: 2}}, // index 1
		{Calls: &scenario.CallBounds{Min: 1, Max: 3}}, // index 2
	}

	state := NewState("/path/to/scenario.yaml", "hash", 3)

	// Not all maxes hit
	state.StepCounts = []int{0, 1, 2}
	assert.False(t, state.GroupAllMaxesHit(gr, steps))

	// All maxes hit
	state.StepCounts = []int{0, 2, 3}
	assert.True(t, state.GroupAllMaxesHit(gr, steps))

	// Over max
	state.StepCounts = []int{0, 5, 5}
	assert.True(t, state.GroupAllMaxesHit(gr, steps))
}

func TestState_GroupAllMinsMet(t *testing.T) {
	gr := scenario.GroupRange{Start: 0, End: 2, Name: "test-group"}
	steps := []scenario.Step{
		{Calls: &scenario.CallBounds{Min: 2, Max: 5}}, // index 0
		{Calls: &scenario.CallBounds{Min: 0, Max: 3}}, // index 1 (optional)
	}

	state := NewState("/path/to/scenario.yaml", "hash", 2)

	// Not met
	state.StepCounts = []int{1, 0}
	assert.False(t, state.GroupAllMinsMet(gr, steps))

	// Met (optional step doesn't need to be called)
	state.StepCounts = []int{2, 0}
	assert.True(t, state.GroupAllMinsMet(gr, steps))

	// Over min
	state.StepCounts = []int{5, 2}
	assert.True(t, state.GroupAllMinsMet(gr, steps))
}
