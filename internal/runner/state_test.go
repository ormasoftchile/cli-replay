package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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

			// Should be in temp directory
			assert.Contains(t, path, os.TempDir())

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
