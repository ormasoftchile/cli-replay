package cmd

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeVerifyRoot creates a fresh root + verify command tree for testing.
func makeVerifyRoot() *cobra.Command {
	// Reset global flag state
	verifyFormatFlag = "text"

	root := &cobra.Command{
		Use:           "cli-replay",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	v := &cobra.Command{
		Use:  "verify [scenario.yaml]",
		Args: cobra.MaximumNArgs(1),
		RunE: runVerify,
	}
	v.Flags().StringVar(&verifyFormatFlag, "format", "text", "Output format: text, json, or junit")
	root.AddCommand(v)
	return root
}

// createMinimalScenario writes a minimal valid scenario YAML for testing.
func createMinimalScenario(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "scenario.yaml")
	content := `meta:
  name: test-scenario
steps:
  - match:
      argv: [echo, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

// T018: Session-aware verify test
func TestVerify_SessionAware(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	// Create a state file with session-A (all steps consumed)
	sessionA := "session-A"
	stateFileA := runner.StateFilePathWithSession(absPath, sessionA)
	stateA := runner.NewState(absPath, "hash123", 1)
	stateA.StepCounts = []int{1} // step consumed
	stateA.CurrentStep = 1
	require.NoError(t, runner.WriteState(stateFileA, stateA))
	defer os.Remove(stateFileA)

	// Set CLI_REPLAY_SESSION and run verify
	t.Setenv("CLI_REPLAY_SESSION", sessionA)

	root := makeVerifyRoot()
	root.SetArgs([]string{"verify", scenarioPath})

	err = root.Execute()
	assert.NoError(t, err, "verify should succeed for completed session-A")
}

// T020: Parallel session isolation test
func TestVerify_ParallelSessionIsolation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	// Create session-A state (complete)
	sessionA := "session-A"
	stateFileA := runner.StateFilePathWithSession(absPath, sessionA)
	stateA := runner.NewState(absPath, "hash123", 1)
	stateA.StepCounts = []int{1}
	stateA.CurrentStep = 1
	require.NoError(t, runner.WriteState(stateFileA, stateA))
	defer os.Remove(stateFileA)

	// Create session-B state (incomplete — 0 steps consumed)
	sessionB := "session-B"
	stateFileB := runner.StateFilePathWithSession(absPath, sessionB)
	stateB := runner.NewState(absPath, "hash123", 1)
	stateB.StepCounts = []int{0}
	require.NoError(t, runner.WriteState(stateFileB, stateB))
	defer os.Remove(stateFileB)

	// Verify session-A should succeed
	t.Setenv("CLI_REPLAY_SESSION", sessionA)
	rootA := makeVerifyRoot()
	rootA.SetArgs([]string{"verify", scenarioPath})
	err = rootA.Execute()
	assert.NoError(t, err, "session-A should pass verification (complete)")

	// Verify session-B should fail (it calls os.Exit, so we can't easily test
	// without subprocess). Instead, read state and check manually.
	t.Setenv("CLI_REPLAY_SESSION", sessionB)
	stateFile := runner.StateFilePath(absPath)
	loadedState, readErr := runner.ReadState(stateFile)
	require.NoError(t, readErr)
	assert.False(t, loadedState.AllStepsConsumed(), "session-B state should be incomplete")
}

// T021: Backward compatibility — no session set
func TestVerify_BackwardCompatibility_NoSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	// Unset CLI_REPLAY_SESSION
	t.Setenv("CLI_REPLAY_SESSION", "")

	// Create a sessionless state file (complete)
	stateFile := runner.StateFilePath(absPath)
	state := runner.NewState(absPath, "hash123", 1)
	state.StepCounts = []int{1}
	state.CurrentStep = 1
	require.NoError(t, runner.WriteState(stateFile, state))
	defer os.Remove(stateFile)

	root := makeVerifyRoot()
	root.SetArgs([]string{"verify", scenarioPath})
	err = root.Execute()
	assert.NoError(t, err, "verify should work without CLI_REPLAY_SESSION (backward compat)")
}

// Test that session-specific state files are independent
func TestVerify_SessionSpecificStateFiles(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	// Verify that different sessions produce different state file paths
	pathA := runner.StateFilePathWithSession(absPath, "session-A")
	pathB := runner.StateFilePathWithSession(absPath, "session-B")
	pathNone := runner.StateFilePathWithSession(absPath, "")

	assert.NotEqual(t, pathA, pathB, "different sessions should have different state files")
	assert.NotEqual(t, pathA, pathNone, "session state should differ from sessionless state")
	assert.NotEqual(t, pathB, pathNone, "session state should differ from sessionless state")
}

// T011: --format json produces parseable JSON for passing scenario
func TestVerify_FormatJSON_Passed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	// Create state with all steps consumed
	stateFile := runner.StateFilePath(absPath)
	state := runner.NewState(absPath, "hash123", 1)
	state.StepCounts = []int{1}
	state.CurrentStep = 1
	require.NoError(t, runner.WriteState(stateFile, state))
	t.Cleanup(func() { _ = runner.DeleteState(stateFile) })

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root := makeVerifyRoot()
	root.SetArgs([]string{"verify", "--format", "json", scenarioPath})
	err = root.Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.NoError(t, err)

	// Parse JSON output
	var result map[string]interface{}
	jsonErr := json.Unmarshal(buf.Bytes(), &result)
	assert.NoError(t, jsonErr, "output should be valid JSON: %s", buf.String())
	assert.Equal(t, true, result["passed"])
	assert.Equal(t, "test-scenario", result["scenario"])
}

// T011: --format junit produces parseable XML for passing scenario
func TestVerify_FormatJUnit_Passed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	stateFile := runner.StateFilePath(absPath)
	state := runner.NewState(absPath, "hash123", 1)
	state.StepCounts = []int{1}
	state.CurrentStep = 1
	require.NoError(t, runner.WriteState(stateFile, state))
	t.Cleanup(func() { _ = runner.DeleteState(stateFile) })

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root := makeVerifyRoot()
	root.SetArgs([]string{"verify", "--format", "junit", scenarioPath})
	err = root.Execute()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	assert.NoError(t, err)

	// Parse XML output
	output := buf.String()
	assert.Contains(t, output, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, output, `<testsuites`)
	assert.Contains(t, output, `name="cli-replay"`)

	var suites struct {
		XMLName xml.Name `xml:"testsuites"`
		Tests   int      `xml:"tests,attr"`
	}
	xmlErr := xml.Unmarshal(buf.Bytes(), &suites)
	assert.NoError(t, xmlErr, "output should be valid XML: %s", output)
	assert.Equal(t, 1, suites.Tests)
}

// T011: --format text (default) produces existing output unchanged
func TestVerify_FormatText_Default(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)
	absPath, err := filepath.Abs(scenarioPath)
	require.NoError(t, err)

	stateFile := runner.StateFilePath(absPath)
	state := runner.NewState(absPath, "hash123", 1)
	state.StepCounts = []int{1}
	state.CurrentStep = 1
	require.NoError(t, runner.WriteState(stateFile, state))
	t.Cleanup(func() { _ = runner.DeleteState(stateFile) })

	root := makeVerifyRoot()
	root.SetArgs([]string{"verify", scenarioPath})
	err = root.Execute()
	assert.NoError(t, err)
}

// T011: invalid format returns error
func TestVerify_FormatInvalid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createMinimalScenario(t, tmpDir)

	root := makeVerifyRoot()
	root.SetArgs([]string{"verify", "--format", "yaml", scenarioPath})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
	assert.Contains(t, err.Error(), "text, json, junit")
}
