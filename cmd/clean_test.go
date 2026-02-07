package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeCleanRoot creates a fresh root + clean command tree for testing.
func makeCleanRoot() *cobra.Command {
	// Reset package-level flag vars so each test gets a clean slate.
	cleanTTLFlag = ""
	cleanRecursiveFlag = false

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
	cl.Flags().StringVar(&cleanTTLFlag, "ttl", "", "Only clean sessions older than this duration")
	cl.Flags().BoolVar(&cleanRecursiveFlag, "recursive", false, "Walk directory tree for .cli-replay/ dirs")
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

// ──────────────────────────────────────────────────────────────────
// T028: Tests for --ttl, --recursive flags
// ──────────────────────────────────────────────────────────────────

// T028a: --recursive without --ttl should error
func TestClean_RecursiveRequiresTTL(t *testing.T) {
	root := makeCleanRoot()
	root.SetArgs([]string{"clean", "--recursive"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--recursive requires --ttl")
}

// T028b: --ttl with invalid duration should error
func TestClean_InvalidTTL(t *testing.T) {
	root := makeCleanRoot()
	root.SetArgs([]string{"clean", "--ttl", "notaduration", "."})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --ttl value")
}

// T028c: --ttl with negative duration should error
func TestClean_NegativeTTL(t *testing.T) {
	root := makeCleanRoot()
	root.SetArgs([]string{"clean", "--ttl", "-5m", "."})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--ttl must be positive")
}

// T028d: --ttl cleans expired sessions only
func TestClean_TTL_CleansExpiredOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a scenario so we can derive a .cli-replay dir
	scenarioPath := createMinimalScenario(t, tmpDir)

	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	require.NoError(t, os.MkdirAll(cliReplayDir, 0750))

	// Create an "expired" state file (last_updated far in the past)
	expiredState := filepath.Join(cliReplayDir, "cli-replay-expired1.state")
	writeStateJSON(t, expiredState, time.Now().Add(-2*time.Hour))

	// Create a "fresh" state file (last_updated is recent)
	freshState := filepath.Join(cliReplayDir, "cli-replay-fresh1.state")
	writeStateJSON(t, freshState, time.Now())

	root := makeCleanRoot()
	root.SetArgs([]string{"clean", "--ttl", "1h", scenarioPath})
	err := root.Execute()
	require.NoError(t, err)

	// Expired should be gone
	assert.NoFileExists(t, expiredState, "expired state should be cleaned")
	// Fresh should remain
	assert.FileExists(t, freshState, "fresh state should remain")
}

// T028e: --recursive walks directories and cleans
func TestClean_Recursive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two project dirs with .cli-replay subdirs
	for _, sub := range []string{"projectA", "projectB"} {
		dir := filepath.Join(tmpDir, sub, ".cli-replay")
		require.NoError(t, os.MkdirAll(dir, 0750))
		expiredFile := filepath.Join(dir, "cli-replay-old.state")
		writeStateJSON(t, expiredFile, time.Now().Add(-3*time.Hour))
	}

	root := makeCleanRoot()
	root.SetArgs([]string{"clean", "--ttl", "1h", "--recursive", tmpDir})
	err := root.Execute()
	require.NoError(t, err)

	// Both expired files should be gone
	assert.NoFileExists(t, filepath.Join(tmpDir, "projectA", ".cli-replay", "cli-replay-old.state"))
	assert.NoFileExists(t, filepath.Join(tmpDir, "projectB", ".cli-replay", "cli-replay-old.state"))
}

// T028f: --recursive skips .git directories
func TestClean_RecursiveSkipsGit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .git/.cli-replay dir that should be skipped
	gitReplay := filepath.Join(tmpDir, ".git", ".cli-replay")
	require.NoError(t, os.MkdirAll(gitReplay, 0750))
	gitState := filepath.Join(gitReplay, "cli-replay-gitold.state")
	writeStateJSON(t, gitState, time.Now().Add(-3*time.Hour))

	// Create a normal .cli-replay dir that should be processed
	normalReplay := filepath.Join(tmpDir, "proj", ".cli-replay")
	require.NoError(t, os.MkdirAll(normalReplay, 0750))
	normalState := filepath.Join(normalReplay, "cli-replay-old.state")
	writeStateJSON(t, normalState, time.Now().Add(-3*time.Hour))

	root := makeCleanRoot()
	root.SetArgs([]string{"clean", "--ttl", "1h", "--recursive", tmpDir})
	err := root.Execute()
	require.NoError(t, err)

	// .git state should still exist (skipped)
	assert.FileExists(t, gitState, ".git dir should be skipped")
	// Normal state should be cleaned
	assert.NoFileExists(t, normalState, "normal expired state should be cleaned")
}

// T028g: --recursive with no expired sessions reports appropriately
func TestClean_RecursiveNoExpired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .cli-replay with only fresh states
	dir := filepath.Join(tmpDir, "proj", ".cli-replay")
	require.NoError(t, os.MkdirAll(dir, 0750))
	freshState := filepath.Join(dir, "cli-replay-fresh.state")
	writeStateJSON(t, freshState, time.Now())

	root := makeCleanRoot()
	root.SetArgs([]string{"clean", "--ttl", "1h", "--recursive", tmpDir})
	err := root.Execute()
	require.NoError(t, err)

	// Fresh state should remain
	assert.FileExists(t, freshState, "fresh state should not be cleaned")
}

// ──────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────

// writeStateJSON writes a minimal state JSON file with the given last_updated time.
func writeStateJSON(t *testing.T, path string, lastUpdated time.Time) {
	t.Helper()
	state := map[string]interface{}{
		"scenario_path": "test.yaml",
		"content_hash":  "abc123",
		"total_steps":   1,
		"last_updated":  lastUpdated.Format(time.RFC3339),
		"consumed":      map[string]interface{}{},
	}
	data, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))
}
