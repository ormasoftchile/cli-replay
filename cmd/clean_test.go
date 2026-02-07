package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeCleanRoot creates a fresh root + clean command tree for testing.
func makeCleanRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "cli-replay",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cl := &cobra.Command{
		Use:  "clean [scenario.yaml]",
		Args: cobra.MaximumNArgs(1),
		RunE: runClean,
	}
	root.AddCommand(cl)
	return root
}

// T019: Session-aware clean test
func TestClean_SessionAware(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	sessionB := "session-B"

	// Create intercept dir inside .cli-replay/ next to scenario
	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	require.NoError(t, os.MkdirAll(cliReplayDir, 0750))
	interceptDir, err := os.MkdirTemp(cliReplayDir, "intercept-")
	require.NoError(t, err)

	// Create a state file with session-B
	stateFileB := runner.StateFilePathWithSession(absPath, sessionB)
	stateB := runner.NewState(absPath, "hash123", 1)
	stateB.InterceptDir = interceptDir
	require.NoError(t, runner.WriteState(stateFileB, stateB))

	// Also create a sessionless state to verify it's NOT removed
	stateFileNone := runner.StateFilePathWithSession(absPath, "")
	stateNone := runner.NewState(absPath, "hash123", 1)
	require.NoError(t, runner.WriteState(stateFileNone, stateNone))
	defer os.Remove(stateFileNone)

	// Set CLI_REPLAY_SESSION=session-B and run clean
	t.Setenv("CLI_REPLAY_SESSION", sessionB)

	root := makeCleanRoot()
	root.SetArgs([]string{"clean", scenarioPath})
	err = root.Execute()
	require.NoError(t, err, "clean should succeed for session-B")

	// Session-B state file should be gone
	assert.NoFileExists(t, stateFileB, "session-B state file should be removed")

	// Session-B intercept dir should be gone
	assert.NoDirExists(t, interceptDir, "session-B intercept dir should be removed")

	// Sessionless state file should still exist (not cross-contaminated)
	assert.FileExists(t, stateFileNone, "sessionless state should NOT be affected by session-B clean")
}

// T022: Clean idempotency — no error when state file doesn't exist
func TestClean_Idempotency(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)

	// Don't create any state file — clean should still succeed
	root := makeCleanRoot()
	root.SetArgs([]string{"clean", scenarioPath})
	err := root.Execute()

	// clean should not error when there's nothing to clean
	assert.NoError(t, err, "clean should be idempotent — no error for missing state")
}

// Test clean removes only the correct session's state
func TestClean_DoesNotAffectOtherSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	// Create state files for two sessions
	stateA := runner.StateFilePathWithSession(absPath, "session-A")
	stateB := runner.StateFilePathWithSession(absPath, "session-B")

	require.NoError(t, runner.WriteState(stateA, runner.NewState(absPath, "h", 1)))
	require.NoError(t, runner.WriteState(stateB, runner.NewState(absPath, "h", 1)))
	defer os.Remove(stateA)
	defer os.Remove(stateB)

	// Clean session-A only
	t.Setenv("CLI_REPLAY_SESSION", "session-A")
	root := makeCleanRoot()
	root.SetArgs([]string{"clean", scenarioPath})
	err = root.Execute()
	require.NoError(t, err)

	// Session-A state should be gone
	assert.NoFileExists(t, stateA, "session-A state should be removed")

	// Session-B state should still exist
	assert.FileExists(t, stateB, "session-B state should NOT be affected")
}

// Test clean with scenario from CLI_REPLAY_SCENARIO env var
func TestClean_UsesEnvVar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	// Create state file
	stateFile := runner.StateFilePath(absPath)
	require.NoError(t, runner.WriteState(stateFile, runner.NewState(absPath, "h", 1)))

	// Set env var instead of passing as arg
	t.Setenv("CLI_REPLAY_SCENARIO", scenarioPath)
	t.Setenv("CLI_REPLAY_SESSION", "")

	root := makeCleanRoot()
	root.SetArgs([]string{"clean"}) // no file arg
	err = root.Execute()
	require.NoError(t, err, "clean should work with CLI_REPLAY_SCENARIO env var")
}
