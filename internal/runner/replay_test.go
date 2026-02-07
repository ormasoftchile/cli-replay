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
	assert.Contains(t, sc.Steps[0].Step.Match.Stdin, "apiVersion: v1")

	// Step 2 should have stdin
	assert.Equal(t, "hello world\n", sc.Steps[1].Step.Match.Stdin)

	// Step 3 should have no stdin
	assert.Empty(t, sc.Steps[2].Step.Match.Stdin)
}

// T027: Unordered group replay tests

func TestExecuteReplay_GroupAnyOrderMatching(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: group-any-order
steps:
  - group:
      mode: unordered
      name: pre-flight
      steps:
        - match:
            argv: ["check", "a"]
          respond:
            exit: 0
            stdout: "a ok\n"
        - match:
            argv: ["check", "b"]
          respond:
            exit: 0
            stdout: "b ok\n"
        - match:
            argv: ["check", "c"]
          respond:
            exit: 0
            stdout: "c ok\n"
  - match:
      argv: ["deploy"]
    respond:
      exit: 0
      stdout: "deployed\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Call in REVERSE order (c, b, a) — all should match within the group
	for _, arg := range []string{"c", "b", "a"} {
		var stdout, stderr bytes.Buffer
		result, rErr := ExecuteReplay(scenarioPath, []string{"check", arg}, &stdout, &stderr)
		require.NoError(t, rErr, "check %s", arg)
		assert.Equal(t, 0, result.ExitCode)
		assert.Contains(t, stdout.String(), arg+" ok")
	}

	// After group exhausted, ordered step should match
	var stdout, stderr bytes.Buffer
	result, rErr := ExecuteReplay(scenarioPath, []string{"deploy"}, &stdout, &stderr)
	require.NoError(t, rErr)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, stdout.String(), "deployed")
}

func TestExecuteReplay_GroupBarrierBlocksOrderedStep(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: group-barrier
steps:
  - group:
      mode: unordered
      name: checks
      steps:
        - match:
            argv: ["check", "a"]
          respond:
            exit: 0
        - match:
            argv: ["check", "b"]
          respond:
            exit: 0
  - match:
      argv: ["deploy"]
    respond:
      exit: 0
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Only call check a (b not yet consumed, min not met)
	var stdout1, stderr1 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"check", "a"}, &stdout1, &stderr1)
	require.NoError(t, err)

	// Try deploy — should fail (group barrier: b not consumed yet)
	var stdout2, stderr2 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"deploy"}, &stdout2, &stderr2)
	require.Error(t, err)
	var gErr *GroupMismatchError
	require.ErrorAs(t, err, &gErr)
	assert.Equal(t, "checks", gErr.GroupName)
	assert.Equal(t, []string{"deploy"}, gErr.Received)
}

func TestExecuteReplay_GroupCallBoundsPerStep(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: group-call-bounds
steps:
  - group:
      mode: unordered
      name: polling
      steps:
        - match:
            argv: ["cmd", "a"]
          calls:
            min: 1
            max: 3
          respond:
            exit: 0
            stdout: "a\n"
        - match:
            argv: ["cmd", "b"]
          calls:
            min: 1
            max: 2
          respond:
            exit: 0
            stdout: "b\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Call a 3 times (max=3)
	for i := 0; i < 3; i++ {
		var stdout, stderr bytes.Buffer
		result, rErr := ExecuteReplay(scenarioPath, []string{"cmd", "a"}, &stdout, &stderr)
		require.NoError(t, rErr, "cmd a call %d", i+1)
		assert.Equal(t, 0, result.ExitCode)
	}

	// Call b 2 times (max=2)
	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		result, rErr := ExecuteReplay(scenarioPath, []string{"cmd", "b"}, &stdout, &stderr)
		require.NoError(t, rErr, "cmd b call %d", i+1)
		assert.Equal(t, 0, result.ExitCode)
	}

	// Group exhausted — scenario should be complete
	var stdout, stderr bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "a"}, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already complete")
}

func TestExecuteReplay_GroupExhaustionAdvances(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: group-exhaust
steps:
  - group:
      mode: unordered
      steps:
        - match:
            argv: ["cmd", "x"]
          respond:
            exit: 0
  - match:
      argv: ["cmd", "y"]
    respond:
      exit: 0
      stdout: "after group\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Consume the single group step
	var stdout1, stderr1 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "x"}, &stdout1, &stderr1)
	require.NoError(t, err)

	// Group exhausted → should advance to ordered step y
	var stdout2, stderr2 bytes.Buffer
	result, rErr := ExecuteReplay(scenarioPath, []string{"cmd", "y"}, &stdout2, &stderr2)
	require.NoError(t, rErr)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, stdout2.String(), "after group")
}

func TestExecuteReplay_GroupMismatchErrorCandidateList(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: group-candidates
steps:
  - group:
      mode: unordered
      name: my-group
      steps:
        - match:
            argv: ["cmd", "alpha"]
          respond:
            exit: 0
        - match:
            argv: ["cmd", "beta"]
          respond:
            exit: 0
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Send an unknown command
	var stdout, stderr bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "gamma"}, &stdout, &stderr)
	require.Error(t, err)

	var gErr *GroupMismatchError
	require.ErrorAs(t, err, &gErr)
	assert.Equal(t, "my-group", gErr.GroupName)
	assert.Equal(t, []string{"cmd", "gamma"}, gErr.Received)
	assert.Len(t, gErr.Candidates, 2)
	assert.Len(t, gErr.CandidateArgv, 2)
	assert.Equal(t, []string{"cmd", "alpha"}, gErr.CandidateArgv[0])
	assert.Equal(t, []string{"cmd", "beta"}, gErr.CandidateArgv[1])
}

func TestExecuteReplay_GroupAllMinZeroImmediateAdvance(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: group-min-zero
steps:
  - group:
      mode: unordered
      name: optional
      steps:
        - match:
            argv: ["cmd", "opt1"]
          calls:
            min: 0
            max: 3
          respond:
            exit: 0
        - match:
            argv: ["cmd", "opt2"]
          calls:
            min: 0
            max: 3
          respond:
            exit: 0
  - match:
      argv: ["cmd", "next"]
    respond:
      exit: 0
      stdout: "next\n"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Skip group entirely — mins are 0, barrier is immediately satisfiable
	var stdout, stderr bytes.Buffer
	result, rErr := ExecuteReplay(scenarioPath, []string{"cmd", "next"}, &stdout, &stderr)
	require.NoError(t, rErr)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, stdout.String(), "next")
}

func TestExecuteReplay_AdjacentGroups(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: adjacent-groups
steps:
  - group:
      mode: unordered
      name: group-a
      steps:
        - match:
            argv: ["cmd", "a1"]
          respond:
            exit: 0
        - match:
            argv: ["cmd", "a2"]
          respond:
            exit: 0
  - group:
      mode: unordered
      name: group-b
      steps:
        - match:
            argv: ["cmd", "b1"]
          respond:
            exit: 0
        - match:
            argv: ["cmd", "b2"]
          respond:
            exit: 0
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Consume group-a in reverse
	for _, arg := range []string{"a2", "a1"} {
		var stdout, stderr bytes.Buffer
		_, rErr := ExecuteReplay(scenarioPath, []string{"cmd", arg}, &stdout, &stderr)
		require.NoError(t, rErr, "cmd %s", arg)
	}

	// Now in group-b — consume in reverse
	for _, arg := range []string{"b2", "b1"} {
		var stdout, stderr bytes.Buffer
		_, rErr := ExecuteReplay(scenarioPath, []string{"cmd", arg}, &stdout, &stderr)
		require.NoError(t, rErr, "cmd %s", arg)
	}

	// All done
	var stdout, stderr bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cmd", "anything"}, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already complete")
}

func TestExecuteReplay_GroupAsFirstStep(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: group-first
steps:
  - group:
      mode: unordered
      steps:
        - match:
            argv: ["first", "b"]
          respond:
            exit: 0
        - match:
            argv: ["first", "a"]
          respond:
            exit: 0
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Match second declared step first
	var stdout1, stderr1 bytes.Buffer
	result, rErr := ExecuteReplay(scenarioPath, []string{"first", "a"}, &stdout1, &stderr1)
	require.NoError(t, rErr)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, 1, result.StepIndex) // flat index 1 for second group child

	// Match first declared step second
	var stdout2, stderr2 bytes.Buffer
	result2, rErr := ExecuteReplay(scenarioPath, []string{"first", "b"}, &stdout2, &stderr2)
	require.NoError(t, rErr)
	assert.Equal(t, 0, result2.ExitCode)
	assert.Equal(t, 0, result2.StepIndex) // flat index 0 for first group child
}

func TestExecuteReplay_GroupAsLastStep(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: group-last
steps:
  - match:
      argv: ["setup"]
    respond:
      exit: 0
  - group:
      mode: unordered
      steps:
        - match:
            argv: ["cleanup", "a"]
          respond:
            exit: 0
        - match:
            argv: ["cleanup", "b"]
          respond:
            exit: 0
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Ordered step first
	var stdout1, stderr1 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"setup"}, &stdout1, &stderr1)
	require.NoError(t, err)

	// Group in any order
	var stdout2, stderr2 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cleanup", "b"}, &stdout2, &stderr2)
	require.NoError(t, err)

	var stdout3, stderr3 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"cleanup", "a"}, &stdout3, &stderr3)
	require.NoError(t, err)

	// Scenario complete
	var stdout4, stderr4 bytes.Buffer
	_, err = ExecuteReplay(scenarioPath, []string{"anything"}, &stdout4, &stderr4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already complete")
}

// T017: Integration tests for User Story 1 (deny env vars)

func TestReplayResponseWithTemplate_DeniedEnvVarEmptyString(t *testing.T) {
	// Denied env var with no meta.vars base → renders as empty string
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "deny-test",
			Vars: map[string]string{"AWS_KEY": ""},
			Security: &scenario.Security{
				DenyEnvVars: []string{"AWS_*"},
			},
		},
	}
	step := &scenario.Step{
		Match:   scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{Exit: 0, Stdout: "key={{ .AWS_KEY }}|end"},
	}

	t.Setenv("AWS_KEY", "real-secret-value")

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithTemplate(step, scn, "/fake/path/scenario.yaml", &stdout, &stderr)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "key=|end", stdout.String())
}

func TestReplayResponseWithTemplate_AllowedEnvVarRealValue(t *testing.T) {
	// Allowed env var → renders with real env value
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "allow-test",
			Vars: map[string]string{"HOME_VAR": "default"},
			Security: &scenario.Security{
				DenyEnvVars: []string{"AWS_*"},
			},
		},
	}
	step := &scenario.Step{
		Match:   scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{Exit: 0, Stdout: "home={{ .HOME_VAR }}"},
	}

	t.Setenv("HOME_VAR", "/real/home")

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithTemplate(step, scn, "/fake/path/scenario.yaml", &stdout, &stderr)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "home=/real/home", stdout.String())
}

func TestReplayResponseWithTemplate_NoSecurityPassthrough(t *testing.T) {
	// No security section → env vars pass through normally (backward compat)
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "no-sec-test",
			Vars: map[string]string{"MY_VAR": "default"},
		},
	}
	step := &scenario.Step{
		Match:   scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{Exit: 0, Stdout: "val={{ .MY_VAR }}"},
	}

	t.Setenv("MY_VAR", "env-override")

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithTemplate(step, scn, "/fake/path/scenario.yaml", &stdout, &stderr)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "val=env-override", stdout.String())
}

func TestReplayResponseWithTemplate_WildcardDenyAll(t *testing.T) {
	// Deny-all with "*" — all env overrides suppressed
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "deny-all-test",
			Vars: map[string]string{
				"VAR_A": "base-a",
				"VAR_B": "base-b",
			},
			Security: &scenario.Security{
				DenyEnvVars: []string{"*"},
			},
		},
	}
	step := &scenario.Step{
		Match:   scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{Exit: 0, Stdout: "a={{ .VAR_A }} b={{ .VAR_B }}"},
	}

	t.Setenv("VAR_A", "env-a")
	t.Setenv("VAR_B", "env-b")

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithTemplate(step, scn, "/fake/path/scenario.yaml", &stdout, &stderr)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "a=base-a b=base-b", stdout.String())
}

func TestReplayResponseWithTemplate_GlobPatterns(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "glob-test",
			Vars: map[string]string{
				"AWS_KEY":      "base-aws",
				"GITHUB_TOKEN": "base-gh",
				"NORMAL":       "base-normal",
			},
			Security: &scenario.Security{
				DenyEnvVars: []string{"AWS_*", "GITHUB_TOKEN"},
			},
		},
	}
	step := &scenario.Step{
		Match:   scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{Exit: 0, Stdout: "{{ .AWS_KEY }}|{{ .GITHUB_TOKEN }}|{{ .NORMAL }}"},
	}

	t.Setenv("AWS_KEY", "env-aws")
	t.Setenv("GITHUB_TOKEN", "env-gh")
	t.Setenv("NORMAL", "env-normal")

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithTemplate(step, scn, "/fake/path/scenario.yaml", &stdout, &stderr)
	assert.Equal(t, 0, exitCode)
	// AWS_KEY and GITHUB_TOKEN denied → base values; NORMAL allowed → env value
	assert.Equal(t, "base-aws|base-gh|env-normal", stdout.String())
}

func TestReplayResponseWithTemplate_DenyWithTrace(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "trace-deny-test",
			Vars: map[string]string{"SECRET": "base"},
			Security: &scenario.Security{
				DenyEnvVars: []string{"SECRET"},
			},
		},
	}
	step := &scenario.Step{
		Match:   scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{Exit: 0, Stdout: "{{ .SECRET }}"},
	}

	t.Setenv("SECRET", "real-secret")
	t.Setenv("CLI_REPLAY_TRACE", "1")

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithTemplate(step, scn, "/fake/path/scenario.yaml", &stdout, &stderr)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "base", stdout.String())
	assert.Contains(t, stderr.String(), "cli-replay[trace]: denied env var SECRET")
}

func TestReplayResponseWithTemplate_DenyPreservesMetaVarsValue(t *testing.T) {
	// When a denied var has a base value in meta.vars, it keeps that value
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "preserve-test",
			Vars: map[string]string{"AWS_KEY": "safe-default-value"},
			Security: &scenario.Security{
				DenyEnvVars: []string{"AWS_*"},
			},
		},
	}
	step := &scenario.Step{
		Match:   scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{Exit: 0, Stdout: "key={{ .AWS_KEY }}"},
	}

	t.Setenv("AWS_KEY", "real-secret")

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithTemplate(step, scn, "/fake/path/scenario.yaml", &stdout, &stderr)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "key=safe-default-value", stdout.String())
}

// T033: Composability test — both deny_env_vars and session.ttl configured simultaneously.
func TestReplayResponseWithTemplate_DenyAndSessionTTL_Composability(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "composability-test",
			Vars: map[string]string{"cluster": "prod"},
			Security: &scenario.Security{
				DenyEnvVars: []string{"SECRET_*"},
			},
			Session: &scenario.Session{
				TTL: "10m",
			},
		},
	}
	step := &scenario.Step{
		Match:   scenario.Match{Argv: []string{"cmd"}},
		Respond: scenario.Response{Exit: 0, Stdout: "cluster={{ .cluster }}"},
	}

	// Set env vars — SECRET_TOKEN should be denied, but cluster should work
	t.Setenv("SECRET_TOKEN", "super-secret")
	t.Setenv("cluster", "override-cluster")

	var stdout, stderr bytes.Buffer
	exitCode := ReplayResponseWithTemplate(step, scn, "/fake/path/scenario.yaml", &stdout, &stderr)
	assert.Equal(t, 0, exitCode)
	// cluster is NOT denied, env override applies
	assert.Equal(t, "cluster=override-cluster", stdout.String())
	// Verify session TTL is parseable (model validation)
	assert.NotNil(t, scn.Meta.Session)
	assert.Equal(t, "10m", scn.Meta.Session.TTL)
}
