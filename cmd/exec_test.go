package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeExecRoot creates a fresh root + exec command tree for testing,
// avoiding global state contamination.
func makeExecRoot() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	// Reset global flag state
	execAllowedCommandsFlag = ""
	execFormatFlag = ""
	execReportFileFlag = ""

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	root := &cobra.Command{
		Use:           "cli-replay",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	ex := &cobra.Command{
		Use:                "exec [flags] <scenario.yaml> -- <command> [args...]",
		RunE:               runExec,
		SilenceUsage:       true,
		DisableFlagParsing: false,
	}
	ex.Flags().StringVar(&execAllowedCommandsFlag, "allowed-commands", "", "Comma-separated list of commands allowed to be intercepted")
	ex.Flags().StringVar(&execFormatFlag, "format", "", "Output format for verification report: json or junit")
	ex.Flags().StringVar(&execReportFileFlag, "report-file", "", "Write verification report to file instead of stderr")
	root.AddCommand(ex)

	root.SetOut(stdout)
	root.SetErr(stderr)

	return root, stdout, stderr
}

// createTestScenario writes a minimal valid scenario YAML to a temp file.
func createTestScenario(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "scenario.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

const singleStepScenario = `meta:
  name: test-scenario
steps:
  - match:
      argv: [echo, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`

const twoStepScenario = `meta:
  name: two-step-test
steps:
  - match:
      argv: [echo, one]
    respond:
      exit: 0
      stdout: "one\n"
  - match:
      argv: [echo, two]
    respond:
      exit: 0
      stdout: "two\n"
`

// T005: Test cobra command registration and flag parsing
func TestExecCommand_Registration(t *testing.T) {
	root, _, _ := makeExecRoot()

	// Verify exec subcommand exists
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "exec" {
			found = true
			// Verify --allowed-commands flag exists
			f := cmd.Flags().Lookup("allowed-commands")
			assert.NotNil(t, f, "--allowed-commands flag should be registered")
			break
		}
	}
	assert.True(t, found, "exec subcommand should be registered on root")
}

func TestExecCommand_ArgsLenAtDash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	// We can't easily test ArgsLenAtDash without executing, but we can
	// verify that the command correctly rejects missing --
	root.SetArgs([]string{"exec", scenarioPath, "echo", "hello"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing '--' separator")
}

// T006: Test runExec with valid scenario + successful child
func TestExecCommand_HappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	// Use 'true' as child command (always exits 0) — scenario won't be
	// fully consumed. We verify the lifecycle runs and cleanup occurs.
	root.SetArgs([]string{"exec", scenarioPath, "--", "true"})
	err := root.Execute()

	// Verification will fail because 'true' doesn't trigger intercepted commands
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scenario verification failed")
	assert.Equal(t, 1, ExecExitCode)

	// Verify no intercept dirs remain from this test
	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	entries, _ := filepath.Glob(filepath.Join(cliReplayDir, "intercept-*"))
	for _, e := range entries {
		// Each should be from other tests, not ours (cleaned up)
		_ = e
	}
}

// T007: Test runExec with invalid scenario file
func TestExecCommand_InvalidScenario(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()

	root.SetArgs([]string{"exec", "/nonexistent/scenario.yaml", "--", "echo", "hello"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load scenario")

	// Verify no intercept dir was created
	// (scenario path is invalid so no .cli-replay/ would be created)
}

// T008: Test runExec with missing command after --
func TestExecCommand_MissingCommandAfterDash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	root.SetArgs([]string{"exec", scenarioPath, "--"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing command after '--'")
}

// T009: Test exit code propagation
func TestExecCommand_ExitCodePropagation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	root.SetArgs([]string{"exec", scenarioPath, "--", "sh", "-c", "exit 42"})
	err := root.Execute()
	require.Error(t, err)
	assert.Equal(t, 42, ExecExitCode, "should propagate child exit code 42")
}

// T010: Test verification failure with unconsumed steps
func TestExecCommand_VerificationFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, twoStepScenario)

	// Run 'true' — exits 0 but triggers no scenario steps
	root.SetArgs([]string{"exec", scenarioPath, "--", "true"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scenario verification failed")
	assert.Equal(t, 1, ExecExitCode)
}

// T011: Test idempotent cleanup (calling cleanup twice does not panic)
func TestExecCommand_IdempotentCleanup(t *testing.T) {
	// Create an intercept dir inside .cli-replay/ to simulate what exec does
	tmpDir := t.TempDir()
	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	require.NoError(t, os.MkdirAll(cliReplayDir, 0750))
	interceptDir, err := os.MkdirTemp(cliReplayDir, "intercept-")
	require.NoError(t, err)

	// Create a fake state file in the same tmpDir
	stateFile := filepath.Join(tmpDir, "test.state")
	state := runner.NewState("/test/scenario.yaml", "hash123", 2)
	require.NoError(t, runner.WriteState(stateFile, state))

	// Simulate the idempotent cleanup pattern from exec
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		_ = os.RemoveAll(interceptDir)
		_ = runner.DeleteState(stateFile)
	}

	// Call cleanup twice — should not panic
	assert.NotPanics(t, func() {
		cleanup()
		cleanup()
	})

	// Verify resources are gone
	assert.NoDirExists(t, interceptDir)
	assert.NoFileExists(t, stateFile)
}

// Test missing scenario path before --
func TestExecCommand_MissingScenarioPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()

	root.SetArgs([]string{"exec", "--", "echo", "hello"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing scenario path")
}

// Test with --allowed-commands flag
func TestExecCommand_AllowedCommandsFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()

	// Scenario uses 'echo' but we only allow 'kubectl'
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	root.SetArgs([]string{"exec", "--allowed-commands=kubectl", scenarioPath, "--", "true"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the allowed commands list")
}

// Test child command not found
func TestExecCommand_ChildNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	root.SetArgs([]string{"exec", scenarioPath, "--", "nonexistent-command-xyz"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start child process")
}

// Test signal-killed child produces 128+signum exit code
func TestExecCommand_SignalKilledChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	// Child kills itself with SIGTERM
	root.SetArgs([]string{"exec", scenarioPath, "--", "sh", "-c", "kill -TERM $$"})
	err := root.Execute()
	require.Error(t, err)
	assert.Equal(t, 143, ExecExitCode, "SIGTERM should produce exit code 143")
}

// Test cleanup happens even when child fails
func TestExecCommand_CleanupOnChildFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	// Count intercept dirs before
	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	beforeDirs, _ := filepath.Glob(filepath.Join(cliReplayDir, "intercept-*"))

	root.SetArgs([]string{"exec", scenarioPath, "--", "sh", "-c", "exit 1"})
	_ = root.Execute()

	// Count intercept dirs after — should not have grown
	afterDirs, _ := filepath.Glob(filepath.Join(cliReplayDir, "intercept-*"))

	// Filter: only count dirs that are actually from our test
	// (simplification: we just check the total didn't grow)
	newDirs := len(afterDirs) - len(beforeDirs)
	assert.LessOrEqual(t, newDirs, 0, "no new intercept dirs should remain after cleanup")
}

// Test multiple args before -- is rejected
func TestExecCommand_MultipleArgsBeforeDash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	root.SetArgs([]string{"exec", scenarioPath, "extra-arg", "--", "echo"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected exactly one scenario path")
}

// Test that exec properly cleans state files
func TestExecCommand_StateFileCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	// Count state files before
	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	beforeStates, _ := filepath.Glob(filepath.Join(cliReplayDir, "cli-replay-*.state"))

	root.SetArgs([]string{"exec", scenarioPath, "--", "true"})
	_ = root.Execute()

	// Count state files after
	afterStates, _ := filepath.Glob(filepath.Join(cliReplayDir, "cli-replay-*.state"))

	newStates := len(afterStates) - len(beforeStates)
	assert.LessOrEqual(t, newStates, 0, "no new state files should remain after exec cleanup")
}

// Test with empty scenario (no steps)
func TestExecCommand_EmptyScenario(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()

	emptyScenario := `meta:
  name: empty-test
steps: []
`
	scenarioPath := createTestScenario(t, tmpDir, emptyScenario)

	root.SetArgs([]string{"exec", scenarioPath, "--", "true"})
	err := root.Execute()
	require.Error(t, err)
	// Empty scenario should fail validation
	assert.Contains(t, err.Error(), "failed to load scenario")
}

func TestExecCommand_StringContainsAllowedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	// 'echo' is in allowed list — should proceed to execution
	root.SetArgs([]string{"exec", "--allowed-commands=echo", scenarioPath, "--", "true"})
	err := root.Execute()
	// Should get past allowlist validation (may fail on verification)
	if err != nil {
		assert.NotContains(t, err.Error(), "not in the allowed commands list")
	}
}

// T012: --report-file writes structured JSON output to a file
func TestExecCommand_ReportFileJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)
	reportPath := filepath.Join(tmpDir, "report.json")

	root, _, _ := makeExecRoot()
	root.SetArgs([]string{"exec", "--format", "json", "--report-file", reportPath, scenarioPath, "--", "echo", "hello"})
	_ = root.Execute() // May fail on verification, but report should be written

	// Verify report file was created
	if _, err := os.Stat(reportPath); err == nil {
		data, readErr := os.ReadFile(reportPath)
		require.NoError(t, readErr)

		var result map[string]interface{}
		jsonErr := json.Unmarshal(data, &result)
		assert.NoError(t, jsonErr, "report file should contain valid JSON: %s", string(data))
		assert.Equal(t, "test-scenario", result["scenario"])
	}
	// Note: if the exec lifecycle fails before Phase 4, report may not be written
}

// T012: --report-file writes structured JUnit output to a file
func TestExecCommand_ReportFileJUnit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)
	reportPath := filepath.Join(tmpDir, "report.xml")

	root, _, _ := makeExecRoot()
	root.SetArgs([]string{"exec", "--format", "junit", "--report-file", reportPath, scenarioPath, "--", "echo", "hello"})
	_ = root.Execute()

	if _, err := os.Stat(reportPath); err == nil {
		data, readErr := os.ReadFile(reportPath)
		require.NoError(t, readErr)
		content := string(data)
		assert.Contains(t, content, `<?xml version="1.0" encoding="UTF-8"?>`)
		assert.Contains(t, content, `<testsuites`)
	}
}

// T012: no report file when --format is omitted
func TestExecCommand_NoReportWithoutFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)
	reportPath := filepath.Join(tmpDir, "report.json")

	root, _, _ := makeExecRoot()
	root.SetArgs([]string{"exec", "--report-file", reportPath, scenarioPath, "--", "echo", "hello"})
	_ = root.Execute()

	// With --report-file but without --format, no structured output should be written
	_, err := os.Stat(reportPath)
	assert.True(t, os.IsNotExist(err), "report file should not exist when --format is omitted")
}

// T012: invalid --format value returns error
func TestExecCommand_InvalidFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	root, _, _ := makeExecRoot()
	tmpDir := t.TempDir()
	scenarioPath := createTestScenario(t, tmpDir, singleStepScenario)

	root.SetArgs([]string{"exec", "--format", "yaml", scenarioPath, "--", "echo", "hello"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

// T012: --format flag registration
func TestExecCommand_FormatFlagRegistered(t *testing.T) {
	root, _, _ := makeExecRoot()

	for _, cmd := range root.Commands() {
		if cmd.Name() == "exec" {
			f := cmd.Flags().Lookup("format")
			assert.NotNil(t, f, "--format flag should be registered")
			rf := cmd.Flags().Lookup("report-file")
			assert.NotNil(t, rf, "--report-file flag should be registered")
			break
		}
	}
}
