package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// executeRecordCmd creates a fresh record command and executes it with the given args.
// This avoids global state contamination between tests.
func executeRecordCmd(args []string) (*bytes.Buffer, *bytes.Buffer, error) { //nolint:unparam // stdout kept for symmetry
	// Reset global flag variables to avoid state leaking between tests
	recordOutputPath = ""
	recordName = ""
	recordDescription = ""
	recordCommands = nil

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// Build a fresh root command for testing
	root := &cobra.Command{
		Use:           "cli-replay",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rec := &cobra.Command{
		Use:  "record [flags] -- <command> [args...]",
		Args: cobra.MinimumNArgs(1),
		RunE: runRecord,
	}
	rec.Flags().StringVarP(&recordOutputPath, "output", "o", "", "output YAML file path")
	rec.Flags().StringVarP(&recordName, "name", "n", "", "scenario name")
	rec.Flags().StringVarP(&recordDescription, "description", "d", "", "scenario description")
	rec.Flags().StringSliceVarP(&recordCommands, "command", "c", []string{}, "commands to intercept")
	_ = rec.MarkFlagRequired("output")
	root.AddCommand(rec)

	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	err := root.Execute()
	return stdout, stderr, err
}

// --- User Story 1: Basic Command Recording ---

func TestRecordCommand_SingleCommand(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.yaml")

	// On Windows, "echo" is a cmd.exe builtin — use cmd /C echo
	var cmdArgs []string
	if runtime.GOOS == "windows" {
		cmdArgs = []string{"record", "--output", outputPath, "--", "cmd", "/C", "echo hello world"}
	} else {
		cmdArgs = []string{"record", "--output", outputPath, "--", "echo", "hello world"}
	}

	_, stderr, err := executeRecordCmd(cmdArgs)
	require.NoError(t, err, "record command should succeed; stderr: %s", stderr.String())

	// Verify YAML file was created
	assert.FileExists(t, outputPath)

	// Parse and validate YAML
	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err, "generated YAML should be valid")

	// Verify scenario structure
	require.Len(t, sc.Steps, 1, "should have exactly one step")
	assert.Contains(t, sc.Steps[0].Step.Respond.Stdout, "hello world")
	assert.Equal(t, 0, sc.Steps[0].Step.Respond.Exit)
	assert.Empty(t, sc.Steps[0].Step.Respond.Stderr)

	// Verify metadata has a name (auto-generated if not specified)
	assert.NotEmpty(t, sc.Meta.Name)
}

func TestRecordCommand_WithMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "named-scenario.yaml")

	var cmdArgs []string
	if runtime.GOOS == "windows" {
		cmdArgs = []string{"record", "--output", outputPath, "--name", "my-test", "--description", "Test scenario", "--", "cmd", "/C", "echo test"}
	} else {
		cmdArgs = []string{"record", "--output", outputPath, "--name", "my-test", "--description", "Test scenario", "--", "echo", "test"}
	}

	_, _, err := executeRecordCmd(cmdArgs)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	assert.Equal(t, "my-test", sc.Meta.Name)
	assert.Equal(t, "Test scenario", sc.Meta.Description)
	require.Len(t, sc.Steps, 1)
	assert.Contains(t, sc.Steps[0].Step.Respond.Stdout, "test")
}

func TestRecordCommand_NonZeroExit(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "error-scenario.yaml")

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"record", "--output", outputPath, "--", "cmd", "/C", "exit 42"}
	} else {
		args = []string{"record", "--output", outputPath, "--", "sh", "-c", "exit 42"}
	}

	_, _, err := executeRecordCmd(args)
	// Record succeeds even with non-zero exit (it captures the failure)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	require.Len(t, sc.Steps, 1)
	assert.Equal(t, 42, sc.Steps[0].Step.Respond.Exit)
}

func TestRecordCommand_StderrCapture(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "stderr-scenario.yaml")

	var args []string
	if runtime.GOOS == "windows" {
		// On Windows, use a small PowerShell script to write to stderr
		args = []string{"record", "--output", outputPath, "--",
			"powershell", "-NoProfile", "-Command",
			"[Console]::Error.WriteLine('errout'); exit 1"}
	} else {
		args = []string{"record", "--output", outputPath, "--",
			"sh", "-c", "echo errout >&2; exit 1"}
	}

	_, _, err := executeRecordCmd(args)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	require.Len(t, sc.Steps, 1)
	assert.Equal(t, 1, sc.Steps[0].Step.Respond.Exit)
	assert.Contains(t, sc.Steps[0].Step.Respond.Stderr, "errout")
}

func TestRecordCommand_MissingOutputFlag(t *testing.T) {
	_, _, err := executeRecordCmd([]string{
		"record", "--", "echo", "test",
	})
	assert.Error(t, err, "should fail when --output is not provided")
}

func TestRecordCommand_CommandNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "notfound.yaml")

	_, _, err := executeRecordCmd([]string{
		"record", "--output", outputPath, "--", "nonexistent-command-12345",
	})
	require.Error(t, err, "should fail when command is not found")
	assert.Contains(t, err.Error(), "failed to execute user command")
}

func TestRecordCommand_InvalidOutputPath(t *testing.T) {
	// Use a path that won't exist on any platform
	badPath := filepath.Join("Z:", "nonexistent", "path", "output.yaml")
	if runtime.GOOS != "windows" {
		badPath = "/nonexistent/path/output.yaml"
	}

	_, _, err := executeRecordCmd([]string{
		"record", "--output", badPath, "--", "echo", "test",
	})
	require.Error(t, err, "should fail when output directory does not exist")
	assert.Contains(t, err.Error(), "output directory does not exist")
}

func TestRecordCommand_OverwriteExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "existing.yaml")

	// Create an existing file
	err := os.WriteFile(outputPath, []byte("old content"), 0600)
	require.NoError(t, err)

	// Record should overwrite
	var overwriteArgs []string
	if runtime.GOOS == "windows" {
		overwriteArgs = []string{"record", "--output", outputPath, "--", "cmd", "/C", "echo new content"}
	} else {
		overwriteArgs = []string{"record", "--output", outputPath, "--", "echo", "new content"}
	}
	_, _, err = executeRecordCmd(overwriteArgs)
	require.NoError(t, err)

	// Verify new content
	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)
	assert.Contains(t, string(content), "new content")
	assert.NotContains(t, string(content), "old content")
}

func TestRecordCommand_EmptyOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "empty-output.yaml")

	// Command that produces no output — use platform-appropriate noop
	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"record", "--output", outputPath, "--", "cmd", "/C", "rem"}
	} else {
		args = []string{"record", "--output", outputPath, "--", "true"}
	}

	_, _, err := executeRecordCmd(args)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	require.Len(t, sc.Steps, 1)
	assert.Equal(t, 0, sc.Steps[0].Step.Respond.Exit)
	assert.Empty(t, sc.Steps[0].Step.Respond.Stdout)
	assert.Empty(t, sc.Steps[0].Step.Respond.Stderr)
}

// --- User Story 2: Multi-Step Workflow ---

func TestRecordCommand_MultiStepWorkflow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash-based test; Windows equivalent covered by PowerShell tests")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "multi-step.yaml")

	// Create a script that runs multiple commands
	script := filepath.Join(tmpDir, "workflow.sh")
	err := os.WriteFile(script, []byte("#!/bin/bash\necho step1\necho step2\necho step3\n"), 0755) //nolint:gosec // test script needs executable permission
	require.NoError(t, err)

	// Direct capture mode records the script as a single command
	_, _, err = executeRecordCmd([]string{
		"record", "--output", outputPath, "--", "bash", script,
	})
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	// In direct capture, the entire bash script is one step
	require.Len(t, sc.Steps, 1)
	assert.Contains(t, sc.Steps[0].Step.Respond.Stdout, "step1")
	assert.Contains(t, sc.Steps[0].Step.Respond.Stdout, "step2")
	assert.Contains(t, sc.Steps[0].Step.Respond.Stdout, "step3")
}

func TestRecordCommand_MultiCommandBashC(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash-based test; Windows equivalent covered by PowerShell tests")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "bash-c.yaml")

	_, _, err := executeRecordCmd([]string{
		"record", "--output", outputPath,
		"--", "bash", "-c", "echo first && echo second && echo third",
	})
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	require.Len(t, sc.Steps, 1)
	assert.Equal(t, "first\nsecond\nthird\n", sc.Steps[0].Step.Respond.Stdout)
	assert.Equal(t, 0, sc.Steps[0].Step.Respond.Exit)
}

// TestRecordCommand_ShimBasedRecording tests the shim interception path
// using --command filter. When filters are specified, shims are generated
// and PATH is modified to intercept those specific commands.
// Note: bash builtins (echo, cd, etc.) cannot be shimmed — only external commands.
func TestRecordCommand_ShimBasedRecording(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash-based shim test; Windows shim tests in platform/windows_test.go")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "shim-test.yaml")

	// Create a test file for cat to read
	testFile := filepath.Join(tmpDir, "input.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("captured-via-shim\n"), 0600))

	// Script that calls 'cat' (an external command, not a builtin)
	script := filepath.Join(tmpDir, "shim-workflow.sh")
	scriptContent := fmt.Sprintf("#!/bin/bash\ncat %s\n", testFile)
	err := os.WriteFile(script, []byte(scriptContent), 0755) //nolint:gosec // test script
	require.NoError(t, err)

	_, _, err = executeRecordCmd([]string{
		"record", "--output", outputPath,
		"--command", "cat",
		"--", "bash", script,
	})
	require.NoError(t, err)

	// Verify YAML was created
	assert.FileExists(t, outputPath)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	// The shim should have intercepted cat
	require.GreaterOrEqual(t, len(sc.Steps), 1, "at least one command should be captured via shim")

	// The captured command should have 'cat' in its argv
	assert.Equal(t, "cat", sc.Steps[0].Step.Match.Argv[0])
	assert.Contains(t, sc.Steps[0].Step.Respond.Stdout, "captured-via-shim")
}

// TestRecordCommand_ShimMultipleCommands tests shim-based recording with
// multiple intercepted commands in a single script execution.
func TestRecordCommand_ShimMultipleCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash-based shim test; Windows shim tests in platform/windows_test.go")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "multi-shim.yaml")

	// Create test files for cat to read
	for i, content := range []string{"first\n", "second\n", "third\n"} {
		f := filepath.Join(tmpDir, fmt.Sprintf("input%d.txt", i+1))
		require.NoError(t, os.WriteFile(f, []byte(content), 0600))
	}

	// Script that runs cat (external command) multiple times
	script := filepath.Join(tmpDir, "multi-shim.sh")
	scriptContent := fmt.Sprintf("#!/bin/bash\ncat %s/input1.txt\ncat %s/input2.txt\ncat %s/input3.txt\n",
		tmpDir, tmpDir, tmpDir)
	err := os.WriteFile(script, []byte(scriptContent), 0755) //nolint:gosec // test script
	require.NoError(t, err)

	_, _, err = executeRecordCmd([]string{
		"record", "--output", outputPath,
		"--command", "cat",
		"--", "bash", script,
	})
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	// Each cat call should be captured as a separate step
	require.Len(t, sc.Steps, 3, "should capture 3 separate cat commands via shims")

	assert.Contains(t, sc.Steps[0].Step.Respond.Stdout, "first")
	assert.Contains(t, sc.Steps[1].Step.Respond.Stdout, "second")
	assert.Contains(t, sc.Steps[2].Step.Respond.Stdout, "third")

	// All should be cat commands
	for i, step := range sc.Steps {
		assert.Equal(t, "cat", step.Step.Match.Argv[0], "step %d should be cat", i)
	}
}

// TestRecordCommand_ShimPreservesExitCodes tests that shim recording properly
// captures exit codes from individual commands.
func TestRecordCommand_ShimPreservesExitCodes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash-based shim test; Windows shim tests in platform/windows_test.go")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "exit-shim.yaml")

	// Create test files
	f1 := filepath.Join(tmpDir, "file1.txt")
	f2 := filepath.Join(tmpDir, "file2.txt")
	require.NoError(t, os.WriteFile(f1, []byte("before\n"), 0600))
	require.NoError(t, os.WriteFile(f2, []byte("after\n"), 0600))

	script := filepath.Join(tmpDir, "exit-test.sh")
	scriptContent := fmt.Sprintf("#!/bin/bash\ncat %s\ncat %s\n", f1, f2)
	err := os.WriteFile(script, []byte(scriptContent), 0755) //nolint:gosec // test script
	require.NoError(t, err)

	_, _, err = executeRecordCmd([]string{
		"record", "--output", outputPath,
		"--command", "cat",
		"--", "bash", script,
	})
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	// Should have 2 cat steps
	require.Len(t, sc.Steps, 2)
	for _, step := range sc.Steps {
		assert.Equal(t, 0, step.Step.Respond.Exit)
	}
}

// --- Validation Tests ---

func TestValidateRecordOutputPath_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	err := validateRecordOutputPath(filepath.Join(tmpDir, "test.yaml"))
	assert.NoError(t, err)
}

func TestValidateRecordOutputPath_Empty(t *testing.T) {
	err := validateRecordOutputPath("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--output flag is required")
}

func TestValidateRecordOutputPath_NonexistentDir(t *testing.T) {
	badPath := "/nonexistent/dir/test.yaml"
	if runtime.GOOS == "windows" {
		badPath = filepath.Join("Z:", "nonexistent", "dir", "test.yaml")
	}
	err := validateRecordOutputPath(badPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output directory does not exist")
}

func TestExtractCommandName(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"empty", []string{}, "unknown"},
		{"simple", []string{"kubectl"}, "kubectl"},
		{"with subcommand", []string{"kubectl", "get", "pods"}, "kubectl get"},
		{"with flag", []string{"kubectl", "--help"}, "kubectl"},
		{"full unix path", []string{"/usr/local/bin/kubectl", "get"}, "kubectl get"},
	}

	if runtime.GOOS == "windows" {
		// Add a Windows-style path test
		tests = append(tests, struct {
			name string
			argv []string
			want string
		}{"full windows path", []string{`C:\Program Files\kubectl.exe`, "get"}, "kubectl.exe get"})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCommandName(tt.argv)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Generated YAML Roundtrip ---

func TestRecordCommand_YAMLRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "roundtrip.yaml")

	var cmdArgs []string
	if runtime.GOOS == "windows" {
		cmdArgs = []string{"record", "--output", outputPath, "--name", "roundtrip-test", "--description", "Test YAML roundtrip", "--", "cmd", "/C", "echo hello world"}
	} else {
		cmdArgs = []string{"record", "--output", outputPath, "--name", "roundtrip-test", "--description", "Test YAML roundtrip", "--", "echo", "hello world"}
	}

	_, _, err := executeRecordCmd(cmdArgs)
	require.NoError(t, err)

	// Read and parse
	content, err := os.ReadFile(outputPath) //nolint:gosec // test file path
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	// Validate against scenario schema
	err = sc.Validate()
	require.NoError(t, err, "generated scenario should pass validation")

	// Re-marshal and verify stability
	remarshaled, err := yaml.Marshal(&sc)
	require.NoError(t, err)

	var sc2 scenario.Scenario
	err = yaml.Unmarshal(remarshaled, &sc2)
	require.NoError(t, err)

	assert.Equal(t, sc.Meta.Name, sc2.Meta.Name)
	assert.Equal(t, sc.Meta.Description, sc2.Meta.Description)
	assert.Len(t, sc2.Steps, len(sc.Steps))
}
