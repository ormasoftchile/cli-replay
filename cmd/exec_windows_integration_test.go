//go:build windows && integration

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildOnce ensures the cli-replay binary is built exactly once per test run.
var (
	buildOnce   sync.Once
	builtBinary string
	buildErr    error
)

// ensureBinary builds cli-replay.exe into a temp directory. The binary is
// shared across all integration tests via sync.Once.
func ensureBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "cli-replay-test-bin-*")
		if err != nil {
			buildErr = fmt.Errorf("MkdirTemp: %w", err)
			return
		}
		builtBinary = filepath.Join(dir, "cli-replay.exe")
		repoRoot := findRepoRoot(t)
		cmd := exec.Command("go", "build", "-o", builtBinary, ".")
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("go build failed: %w\n%s", err, string(out))
		}
	})
	if buildErr != nil {
		t.Fatalf("failed to build cli-replay: %v", buildErr)
	}
	return builtBinary
}

// findRepoRoot walks up from the working directory to find go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod)")
		}
		dir = parent
	}
}

// writeScenario creates a scenario YAML in dir and returns its path.
func writeScenario(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "scenario.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	return p
}

// writeChildScript creates a .cmd child script that invokes the given commands.
// On Windows, CMD built-ins (echo, dir, etc.) are NEVER resolved via PATH, so
// they cannot be intercepted. The scenario must use a custom command name (e.g.
// "myapp") that does not shadow any built-in. The exec command copies
// cli-replay.exe as myapp.exe into the intercept directory and prepends it to
// PATH, so when the child script calls "myapp hello", Windows finds myapp.exe
// (the intercept copy), which enters replay mode.
func writeChildScript(t *testing.T, dir string, commands ...string) string {
	t.Helper()
	p := filepath.Join(dir, "child.cmd")
	var buf bytes.Buffer
	buf.WriteString("@echo off\r\n")
	for _, c := range commands {
		buf.WriteString(c + "\r\n")
		buf.WriteString("if %ERRORLEVEL% NEQ 0 exit /B %ERRORLEVEL%\r\n")
	}
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0644))
	return p
}

// runCLI invokes the built binary with args and returns stdout, stderr,
// and the exit code. Does not fail the test on non-zero exit.
func runCLI(t *testing.T, binary string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running cli-replay: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// runCLIWithEnv invokes the built binary with custom environment variables.
func runCLIWithEnv(t *testing.T, binary string, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running cli-replay: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// ---------- Category 1: Full exec lifecycle ----------

// TestWindows_ExecLifecycle_SingleStep verifies the complete exec lifecycle
// on Windows: load scenario → create intercept → spawn child → child invokes
// intercepted command → verify → cleanup.
func TestWindows_ExecLifecycle_SingleStep(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()

	// Use "myapp" — a non-existent, non-built-in command name. CMD built-ins
	// (echo, dir, type, etc.) are never resolved via PATH, so they bypass the
	// intercept entirely. A custom name forces PATH resolution → intercept dir.
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-single
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`)

	childScript := writeChildScript(t, tmpDir, "myapp hello")

	_, stderr, exitCode := runCLI(t, binary, "exec", scenarioPath, "--", childScript)

	assert.Equal(t, 0, exitCode, "exit code should be 0: stderr=%s", stderr)
	assert.Contains(t, stderr, "completed")
	assert.Contains(t, stderr, "1/1 steps consumed")

	// Verify cleanup: no intercept dirs remain
	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	entries, _ := filepath.Glob(filepath.Join(cliReplayDir, "intercept-*"))
	assert.Empty(t, entries, "intercept dirs should be cleaned up")
}

func TestWindows_ExecLifecycle_MultiStep(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-multi
steps:
  - match:
      argv: [myapp, one]
    respond:
      exit: 0
      stdout: "one"
  - match:
      argv: [myapp, two]
    respond:
      exit: 0
      stdout: "two"
  - match:
      argv: [myapp, three]
    respond:
      exit: 0
      stdout: "three"
`)

	childScript := writeChildScript(t, tmpDir,
		"myapp one", "myapp two", "myapp three")

	_, stderr, exitCode := runCLI(t, binary, "exec", scenarioPath, "--", childScript)

	assert.Equal(t, 0, exitCode, "exit code should be 0: stderr=%s", stderr)
	assert.Contains(t, stderr, "3/3 steps consumed")
}

func TestWindows_ExecLifecycle_VerificationFailure(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-verify-fail
steps:
  - match:
      argv: [myapp, one]
    respond:
      exit: 0
      stdout: "one"
  - match:
      argv: [myapp, two]
    respond:
      exit: 0
      stdout: "two"
`)

	// Only trigger first step — second remains unconsumed
	childScript := writeChildScript(t, tmpDir, "myapp one")

	_, stderr, exitCode := runCLI(t, binary, "exec", scenarioPath, "--", childScript)

	assert.Equal(t, 1, exitCode, "exit code should be 1 for verification failure")
	assert.Contains(t, stderr, "incomplete")
}

func TestWindows_ExecLifecycle_ChildExitCode(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()

	// The intercepted command returns exit 42. The child script propagates it.
	// NOTE: main.go always exits with 1 when any error is returned from
	// runExec, so the process exit code will be 1 — not 42. We verify that
	// the child's exit code is reported in stderr and the process is non-zero.
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-exit-propagate
steps:
  - match:
      argv: [myapp, fail]
    respond:
      exit: 42
      stdout: ""
`)

	childScript := writeChildScript(t, tmpDir, "myapp fail")

	_, stderr, exitCode := runCLI(t, binary, "exec", scenarioPath, "--", childScript)

	assert.NotEqual(t, 0, exitCode, "non-zero exit should propagate")
	assert.Contains(t, stderr, "exited with code 42", "stderr should report the child's exit code")
}

func TestWindows_ExecLifecycle_CleanupAfterFailure(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-cleanup
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello"
`)

	// Child fails — cleanup should still happen
	runCLI(t, binary, "exec", scenarioPath, "--", "cmd", "/c", "exit 1")

	// Verify no state files remain
	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	stateFiles, _ := filepath.Glob(filepath.Join(cliReplayDir, "*.state"))
	assert.Empty(t, stateFiles, "state files should be cleaned up after failure")

	interceptDirs, _ := filepath.Glob(filepath.Join(cliReplayDir, "intercept-*"))
	assert.Empty(t, interceptDirs, "intercept dirs should be cleaned up after failure")
}

// ---------- Category 2: JSON/JUnit report output ----------

func TestWindows_ExecLifecycle_JSONReport(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-json
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`)
	reportPath := filepath.Join(tmpDir, "report.json")

	childScript := writeChildScript(t, tmpDir, "myapp hello")

	_, _, exitCode := runCLI(t, binary, "exec",
		"--format", "json",
		"--report-file", reportPath,
		scenarioPath, "--", childScript)

	assert.Equal(t, 0, exitCode)

	data, err := os.ReadFile(reportPath)
	require.NoError(t, err, "report file should exist")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &result))
	assert.Equal(t, "win-json", result["scenario"])
}

func TestWindows_ExecLifecycle_JUnitReport(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-junit
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`)
	reportPath := filepath.Join(tmpDir, "report.xml")

	childScript := writeChildScript(t, tmpDir, "myapp hello")

	_, _, exitCode := runCLI(t, binary, "exec",
		"--format", "junit",
		"--report-file", reportPath,
		scenarioPath, "--", childScript)

	assert.Equal(t, 0, exitCode)

	data, err := os.ReadFile(reportPath)
	require.NoError(t, err, "report file should exist")

	content := string(data)
	assert.Contains(t, content, "<?xml")
	assert.Contains(t, content, "testsuites")
}

// ---------- Category 3: Concurrent sessions ----------

func TestWindows_ConcurrentSessions_Isolated(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-concurrent
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`)

	childScript := writeChildScript(t, tmpDir, "myapp hello")

	const nParallel = 4
	var wg sync.WaitGroup
	results := make([]int, nParallel)
	errs := make([]string, nParallel)

	for i := 0; i < nParallel; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, stderr, code := runCLI(t, binary, "exec", scenarioPath, "--", childScript)
			results[idx] = code
			errs[idx] = stderr
		}(i)
	}
	wg.Wait()

	successCount := 0
	for i, code := range results {
		if code == 0 {
			successCount++
		} else {
			t.Logf("session %d failed (code %d): %s", i, code, errs[i])
		}
	}
	assert.Greater(t, successCount, 0, "at least one concurrent session should succeed")
}

func TestWindows_ConcurrentSessions_NoStateFileLeak(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-no-leak
steps:
  - match:
      argv: [myapp, hi]
    respond:
      exit: 0
      stdout: "hi\n"
`)

	childScript := writeChildScript(t, tmpDir, "myapp hi")

	const nParallel = 3
	var wg sync.WaitGroup
	for i := 0; i < nParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runCLI(t, binary, "exec", scenarioPath, "--", childScript)
		}()
	}
	wg.Wait()

	time.Sleep(200 * time.Millisecond)

	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	stateFiles, _ := filepath.Glob(filepath.Join(cliReplayDir, "*.state"))
	assert.Empty(t, stateFiles, "no state files should remain after all sessions complete")

	interceptDirs, _ := filepath.Glob(filepath.Join(cliReplayDir, "intercept-*"))
	assert.Empty(t, interceptDirs, "no intercept dirs should remain")
}

// ---------- Category 4: Edge cases — paths ----------

func TestWindows_PathWithSpaces(t *testing.T) {
	binary := ensureBinary(t)

	baseDir := t.TempDir()
	spacedDir := filepath.Join(baseDir, "My Projects", "test scenario")
	require.NoError(t, os.MkdirAll(spacedDir, 0755))

	scenarioPath := writeScenario(t, spacedDir, `
meta:
  name: win-spaces
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`)

	childScript := writeChildScript(t, spacedDir, "myapp hello")

	_, stderr, exitCode := runCLI(t, binary, "exec", scenarioPath, "--", childScript)

	assert.Equal(t, 0, exitCode,
		"should handle paths with spaces: stderr=%s", stderr)
}

func TestWindows_PathWithUnicode(t *testing.T) {
	binary := ensureBinary(t)

	baseDir := t.TempDir()
	unicodeDir := filepath.Join(baseDir, "テスト")
	err := os.MkdirAll(unicodeDir, 0755)
	if err != nil {
		t.Skipf("filesystem does not support Unicode directory names: %v", err)
	}

	scenarioPath := writeScenario(t, unicodeDir, `
meta:
  name: win-unicode
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`)

	childScript := writeChildScript(t, unicodeDir, "myapp hello")

	_, stderr, exitCode := runCLI(t, binary, "exec", scenarioPath, "--", childScript)

	assert.Equal(t, 0, exitCode,
		"should handle Unicode paths: stderr=%s (path valid=%v)",
		stderr, utf8.ValidString(scenarioPath))
}

func TestWindows_LongPath(t *testing.T) {
	binary := ensureBinary(t)

	baseDir := t.TempDir()
	segment := strings.Repeat("a", 50)
	longDir := filepath.Join(baseDir, segment, segment, segment)
	err := os.MkdirAll(longDir, 0755)
	if err != nil {
		t.Skipf("filesystem does not support path length %d: %v", len(longDir), err)
	}

	scenarioPath := writeScenario(t, longDir, `
meta:
  name: win-longpath
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello\n"
`)

	childScript := writeChildScript(t, longDir, "myapp hello")

	_, stderr, exitCode := runCLI(t, binary, "exec", scenarioPath, "--", childScript)

	assert.Equal(t, 0, exitCode,
		"should handle long paths (len=%d): stderr=%s",
		len(scenarioPath), stderr)
}

// ---------- Category 5: Dry-run mode on Windows ----------

func TestWindows_DryRun(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-dryrun
steps:
  - match:
      argv: [kubectl, get, pods]
    respond:
      exit: 0
      stdout: "pods"
  - match:
      argv: [kubectl, apply, -f, deploy.yaml]
    respond:
      exit: 0
      stdout: "applied"
`)

	stdout, _, exitCode := runCLI(t, binary, "exec", "--dry-run",
		scenarioPath, "--", "cmd", "/c", "echo noop")

	assert.Equal(t, 0, exitCode, "dry-run should exit 0")
	assert.Contains(t, stdout, "win-dryrun")
	assert.Contains(t, stdout, "kubectl get pods")
	assert.Contains(t, stdout, "Steps: 2")

	// Verify no side effects
	cliReplayDir := filepath.Join(tmpDir, ".cli-replay")
	_, err := os.Stat(cliReplayDir)
	assert.True(t, os.IsNotExist(err), "dry-run should not create .cli-replay/")
}

// ---------- Category 6: Validate command on Windows ----------

func TestWindows_ValidateCommand(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-validate
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello"
`)

	_, stderr, exitCode := runCLI(t, binary, "validate", scenarioPath)
	assert.Equal(t, 0, exitCode, "validate should pass for valid scenario")
	assert.Contains(t, stderr, "valid")
}

func TestWindows_ValidateCommand_InvalidScenario(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()

	badPath := filepath.Join(tmpDir, "bad.yaml")
	require.NoError(t, os.WriteFile(badPath, []byte(`
meta:
  name: ""
steps: []
`), 0644))

	_, _, exitCode := runCLI(t, binary, "validate", badPath)
	assert.Equal(t, 1, exitCode, "validate should fail for invalid scenario")
}

// ---------- Category 7: Clean command on Windows ----------

func TestWindows_CleanCommand(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-clean
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello"
`)

	// Run exec (will fail verification since child doesn't invoke myapp) to create state
	runCLI(t, binary, "exec", scenarioPath, "--", "cmd", "/c", "exit 0")

	// Clean should succeed even if exec already cleaned up
	_, _, exitCode := runCLI(t, binary, "clean", scenarioPath)
	assert.Equal(t, 0, exitCode, "clean should succeed idempotently")
}

// ---------- Category 8: Allowlist on Windows ----------

func TestWindows_AllowlistBlocked(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	scenarioPath := writeScenario(t, tmpDir, `
meta:
  name: win-allowlist
steps:
  - match:
      argv: [myapp, hello]
    respond:
      exit: 0
      stdout: "hello"
`)

	// Allow only 'kubectl' but scenario uses 'myapp'
	_, stderr, exitCode := runCLI(t, binary, "exec",
		"--allowed-commands=kubectl", scenarioPath, "--",
		"cmd", "/c", "exit 0")

	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "not in the allowed commands list")
}
