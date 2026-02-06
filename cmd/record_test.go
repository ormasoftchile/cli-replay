package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRecordCommand_SingleCommand(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.yaml")

	// This test will verify the end-to-end flow:
	// 1. Execute: cli-replay record --output output.yaml -- echo "hello world"
	// 2. Verify JSONL log is created with correct entry
	// 3. Verify YAML scenario file is generated
	// 4. Verify YAML can be parsed and contains expected data

	// Note: This test requires the full record command implementation
	// It will fail until T014-T027 are implemented

	t.Skip("Implementation pending - requires record command (T023-T026)")

	// TODO: Implement test once record command is ready
	// Expected behavior:
	// - Command execution creates session
	// - Shims intercept echo command
	// - JSONL log captures: timestamp, argv=["echo", "hello world"], exit=0, stdout="hello world\n"
	// - Converter generates scenario with 1 step
	// - YAML file written to outputPath
	// - YAML is valid and can be unmarshaled

	assert.FileExists(t, outputPath)

	// Parse YAML
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	// Verify scenario
	require.Len(t, sc.Steps, 1)
	assert.Equal(t, []string{"echo", "hello world"}, sc.Steps[0].Match.Argv)
	assert.Equal(t, "hello world\n", sc.Steps[0].Respond.Stdout)
	assert.Equal(t, 0, sc.Steps[0].Respond.Exit)
}

func TestRecordCommand_WithMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "named-scenario.yaml")

	// Test: cli-replay record --name "my-test" --description "Test scenario" --output named-scenario.yaml -- echo "test"

	t.Skip("Implementation pending - requires record command with metadata flags")

	// TODO: Verify metadata is captured
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	assert.Equal(t, "my-test", sc.Meta.Name)
	assert.Equal(t, "Test scenario", sc.Meta.Description)
}

func TestRecordCommand_NonZeroExit(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "error-scenario.yaml")

	// Test: cli-replay record --output error-scenario.yaml -- sh -c "exit 42"

	t.Skip("Implementation pending - requires record command")

	// Verify non-zero exit code is captured
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var sc scenario.Scenario
	err = yaml.Unmarshal(content, &sc)
	require.NoError(t, err)

	require.Len(t, sc.Steps, 1)
	assert.Equal(t, 42, sc.Steps[0].Respond.Exit)
}

func TestRecordCommand_MissingOutputFlag(t *testing.T) {
	// Test: cli-replay record -- echo "test"
	// Expected: Error message indicating --output is required

	t.Skip("Implementation pending - requires record command error handling")

	// TODO: Execute command without --output flag
	// Verify error is returned
	// Verify helpful error message
}

func TestRecordCommand_CommandNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "notfound.yaml")

	// Test: cli-replay record --output notfound.yaml -- nonexistent-command

	t.Skip("Implementation pending - requires record command error handling")

	// TODO: Execute with non-existent command
	// Verify error is returned
	// Verify error message mentions command not found
}

func TestRecordCommand_InvalidOutputPath(t *testing.T) {
	// Test: cli-replay record --output /nonexistent/path/output.yaml -- echo "test"

	t.Skip("Implementation pending - requires record command validation")

	// TODO: Execute with invalid output path
	// Verify error is returned before execution
	// Verify helpful error message about directory not existing
}
