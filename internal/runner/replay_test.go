package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cli-replay/cli-replay/internal/scenario"
)

func TestReplayResponse_Stdout(t *testing.T) {
	step := &scenario.Step{
		Match: scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{
			Exit:   0,
			Stdout: "hello world\n",
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponse(step, "", &stdout, &stderr)

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello world\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestReplayResponse_Stderr(t *testing.T) {
	step := &scenario.Step{
		Match: scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{
			Exit:   1,
			Stderr: "error: something went wrong\n",
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponse(step, "", &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "error: something went wrong\n", stderr.String())
}

func TestReplayResponse_BothStdoutAndStderr(t *testing.T) {
	step := &scenario.Step{
		Match: scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{
			Exit:   2,
			Stdout: "partial output\n",
			Stderr: "warning: incomplete\n",
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponse(step, "", &stdout, &stderr)

	assert.Equal(t, 2, exitCode)
	assert.Equal(t, "partial output\n", stdout.String())
	assert.Equal(t, "warning: incomplete\n", stderr.String())
}

func TestReplayResponse_ExitCodeOnly(t *testing.T) {
	step := &scenario.Step{
		Match: scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{
			Exit: 42,
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponse(step, "", &stdout, &stderr)

	assert.Equal(t, 42, exitCode)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestReplayResponse_ExitCode255(t *testing.T) {
	step := &scenario.Step{
		Match: scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{
			Exit: 255,
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponse(step, "", &stdout, &stderr)

	assert.Equal(t, 255, exitCode)
}

func TestReplayResponse_StdoutFile(t *testing.T) {
	// Create temp file with content
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "fixtures")
	require.NoError(t, os.MkdirAll(fixtureDir, 0750))

	fixtureFile := filepath.Join(fixtureDir, "output.txt")
	err := os.WriteFile(fixtureFile, []byte("content from file\n"), 0600)
	require.NoError(t, err)

	// Create scenario file in tmpDir
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")

	step := &scenario.Step{
		Match: scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{
			Exit:       0,
			StdoutFile: "fixtures/output.txt",
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithFile(step, scenarioPath, &stdout, &stderr)

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "content from file\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestReplayResponse_StderrFile(t *testing.T) {
	// Create temp file with content
	tmpDir := t.TempDir()
	fixtureDir := filepath.Join(tmpDir, "fixtures")
	require.NoError(t, os.MkdirAll(fixtureDir, 0750))

	fixtureFile := filepath.Join(fixtureDir, "error.txt")
	err := os.WriteFile(fixtureFile, []byte("error from file\n"), 0600)
	require.NoError(t, err)

	// Create scenario file in tmpDir
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")

	step := &scenario.Step{
		Match: scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{
			Exit:       1,
			StderrFile: "fixtures/error.txt",
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithFile(step, scenarioPath, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "error from file\n", stderr.String())
}

func TestReplayResponse_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")

	step := &scenario.Step{
		Match: scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{
			Exit:       0,
			StdoutFile: "nonexistent.txt",
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithFile(step, scenarioPath, &stdout, &stderr)

	// Should return error exit code
	assert.NotEqual(t, 0, exitCode)
	// Should write error to stderr
	assert.Contains(t, stderr.String(), "failed to read")
}

// T026: Unit tests for step ordering enforcement
func TestReplayResponse_StepOrdering(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a multi-step scenario
	scenarioContent := `
meta:
  name: "ordering-test"
steps:
  - match:
      argv: ["cmd", "step1"]
    respond:
      exit: 0
      stdout: "step1 output"
  - match:
      argv: ["cmd", "step2"]
    respond:
      exit: 0
      stdout: "step2 output"
  - match:
      argv: ["cmd", "step3"]
    respond:
      exit: 0
      stdout: "step3 output"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// First call should expect step1
	var stdout1, stderr1 bytes.Buffer
	result1, err := ExecuteReplay(scenarioPath, []string{"cmd", "step1"}, &stdout1, &stderr1)
	require.NoError(t, err)
	assert.Equal(t, 0, result1.ExitCode)
	assert.Equal(t, 0, result1.StepIndex)
	assert.Contains(t, stdout1.String(), "step1 output")

	// Second call should expect step2
	var stdout2, stderr2 bytes.Buffer
	result2, err := ExecuteReplay(scenarioPath, []string{"cmd", "step2"}, &stdout2, &stderr2)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.ExitCode)
	assert.Equal(t, 1, result2.StepIndex)
	assert.Contains(t, stdout2.String(), "step2 output")

	// Third call should expect step3
	var stdout3, stderr3 bytes.Buffer
	result3, err := ExecuteReplay(scenarioPath, []string{"cmd", "step3"}, &stdout3, &stderr3)
	require.NoError(t, err)
	assert.Equal(t, 0, result3.ExitCode)
	assert.Equal(t, 2, result3.StepIndex)
	assert.Contains(t, stdout3.String(), "step3 output")
}

// T020: Tests for call count replay

func TestExecuteReplay_RepeatedCallsWithinBudget(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: call-budget-test
steps:
  - match:
      argv: ["cmd", "poll"]
    calls:
      min: 1
      max: 3
    respond:
      exit: 0
      stdout: "polling...\n"
  - match:
      argv: ["cmd", "done"]
    respond:
      exit: 0
      stdout: "done\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Call poll 3 times within budget
	for i := 0; i < 3; i++ {
		var stdout, stderr bytes.Buffer
		result, err := ExecuteReplay(scenarioPath, []string{"cmd", "poll"}, &stdout, &stderr)
		require.NoError(t, err, "poll call %d", i+1)
		assert.Equal(t, 0, result.ExitCode)
		assert.Contains(t, stdout.String(), "polling...")
	}

	// Next call should auto-advance to step 2
	var stdout, stderr bytes.Buffer
	result, err := ExecuteReplay(scenarioPath, []string{"cmd", "done"}, &stdout, &stderr)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, stdout.String(), "done")
}

func TestExecuteReplay_AutoAdvanceAtMax(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: auto-advance-test
steps:
  - match:
      argv: ["cmd", "step1"]
    calls:
      min: 1
      max: 2
    respond:
      exit: 0
      stdout: "step1\n"
  - match:
      argv: ["cmd", "step2"]
    respond:
      exit: 0
      stdout: "step2\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Call step1 twice (max=2)
	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		result, err := ExecuteReplay(scenarioPath, []string{"cmd", "step1"}, &stdout, &stderr)
		require.NoError(t, err, "step1 call %d", i+1)
		assert.Equal(t, 0, result.ExitCode)
	}

	// step1 exhausted, should now match step2
	var stdout, stderr bytes.Buffer
	result, err := ExecuteReplay(scenarioPath, []string{"cmd", "step2"}, &stdout, &stderr)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, stdout.String(), "step2")
}

func TestExecuteReplay_SoftAdvanceWhenMinMet(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: soft-advance-test
steps:
  - match:
      argv: ["cmd", "poll"]
    calls:
      min: 1
      max: 5
    respond:
      exit: 0
      stdout: "polling\n"
  - match:
      argv: ["cmd", "done"]
    respond:
      exit: 0
      stdout: "done\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Call poll once (min=1 satisfied)
	var stdout1, stderr1 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "poll"}, &stdout1, &stderr1)
	require.NoError(t, err)

	// Now call "done" — should soft-advance past poll (min met) and match step2
	var stdout2, stderr2 bytes.Buffer
	result, err := ExecuteReplay(scenarioPath, []string{"cmd", "done"}, &stdout2, &stderr2)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, stdout2.String(), "done")
}

func TestExecuteReplay_HardMismatchWhenMinNotMet(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: hard-mismatch-test
steps:
  - match:
      argv: ["cmd", "required"]
    calls:
      min: 2
      max: 5
    respond:
      exit: 0
      stdout: "ok\n"
  - match:
      argv: ["cmd", "next"]
    respond:
      exit: 0
      stdout: "next\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Call required once (min=2 not met)
	var stdout1, stderr1 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "required"}, &stdout1, &stderr1)
	require.NoError(t, err)

	// Try "next" — min not met, should fail with mismatch
	var stdout2, stderr2 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "next"}, &stdout2, &stderr2)
	require.Error(t, err)
	var mErr *MismatchError
	require.ErrorAs(t, err, &mErr)
	assert.Equal(t, []string{"cmd", "required"}, mErr.Expected)
}

func TestExecuteReplay_DefaultExactlyOnce(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: default-once-test
steps:
  - match:
      argv: ["cmd", "step1"]
    respond:
      exit: 0
      stdout: "first\n"
  - match:
      argv: ["cmd", "step2"]
    respond:
      exit: 0
      stdout: "second\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Call step1 once (default exactly once)
	var stdout1, stderr1 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "step1"}, &stdout1, &stderr1)
	require.NoError(t, err)

	// Call step1 again should fail — budget exhausted, auto-advanced to step2
	var stdout2, stderr2 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "step1"}, &stdout2, &stderr2)
	require.Error(t, err)
	var mErr *MismatchError
	require.ErrorAs(t, err, &mErr)
}

// T031: Tests for normalizeStdin

func TestNormalizeStdin(t *testing.T) {
	// Trailing newline stripped
	assert.Equal(t, "hello", normalizeStdin("hello\n"))
	assert.Equal(t, "hello", normalizeStdin("hello\n\n"))

	// CRLF to LF
	assert.Equal(t, "hello\nworld", normalizeStdin("hello\r\nworld\r\n"))

	// Empty
	assert.Empty(t, normalizeStdin(""))
	assert.Empty(t, normalizeStdin("\n"))

	// No trailing newline unchanged
	assert.Equal(t, "hello", normalizeStdin("hello"))
}

// T031: Test that stdin_match.yaml fixture loads and has stdin fields
func TestStdinMatchFixtureLoads(t *testing.T) {
	sc, err := scenario.LoadFile("../../testdata/scenarios/stdin_match.yaml")
	require.NoError(t, err)

	require.Len(t, sc.Steps, 3)

	// Step 1 should have stdin
	assert.Contains(t, sc.Steps[0].Match.Stdin, "apiVersion: v1")

	// Step 2 should have stdin
	assert.Equal(t, "hello world\n", sc.Steps[1].Match.Stdin)

	// Step 3 should have no stdin
	assert.Empty(t, sc.Steps[2].Match.Stdin)
}
