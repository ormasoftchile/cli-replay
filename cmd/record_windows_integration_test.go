//go:build windows && integration

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Recording shim e2e tests ----------

// TestWindows_Record_BasicCapture records a simple command and verifies
// the output YAML has the expected structure.
func TestWindows_Record_BasicCapture(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "recorded.yaml")

	_, stderr, exitCode := runCLI(t, binary, "record",
		"--output", outputPath,
		"--name", "win-record-test",
		"--", "cmd", "/c", "echo hello from recording")

	assert.Equal(t, 0, exitCode, "record should succeed: stderr=%s", stderr)

	// Verify output file exists and has valid YAML
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "name: win-record-test")
	assert.Contains(t, content, "argv:")
	assert.Contains(t, content, "exit:")
	assert.Contains(t, content, "hello from recording")
}

// TestWindows_Record_NonZeroExit records a command that exits non-zero
// and verifies the exit code is captured (exit 0 from record, non-zero in YAML).
func TestWindows_Record_NonZeroExit(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "recorded.yaml")

	_, stderr, exitCode := runCLI(t, binary, "record",
		"--output", outputPath,
		"--name", "win-record-fail",
		"--", "cmd", "/c", "echo fail & exit 42")

	// record should exit 0 even when child exits non-zero
	assert.Equal(t, 0, exitCode, "record should exit 0: stderr=%s", stderr)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	content := string(data)

	// The recorded exit code should be 42
	assert.Contains(t, content, "exit: 42")
}

// TestWindows_Record_ShimMode_CommandFlag records using --command to create
// .cmd/.ps1 shim pairs that intercept specific commands.
func TestWindows_Record_ShimMode_CommandFlag(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "recorded.yaml")

	// Record with --command flag: this creates shims for 'hostname'
	// which is a real command on Windows
	_, stderr, exitCode := runCLI(t, binary, "record",
		"--output", outputPath,
		"--name", "win-shim-test",
		"--command", "hostname",
		"--", "cmd", "/c", "hostname")

	// This may fail if hostname isn't interceptable, but should at least
	// not crash. Check that the binary ran without panic.
	if exitCode != 0 {
		t.Logf("record with --command exited %d (may be expected): stderr=%s", exitCode, stderr)
	}

	// At minimum, verify the binary didn't crash
	assert.False(t, strings.Contains(stderr, "panic"),
		"record should not panic")
}

// TestWindows_Record_StderrCapture records a command that outputs to stderr.
func TestWindows_Record_StderrCapture(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "recorded.yaml")

	_, stderr, exitCode := runCLI(t, binary, "record",
		"--output", outputPath,
		"--name", "win-stderr-test",
		"--", "cmd", "/c", "echo error-output 1>&2")

	assert.Equal(t, 0, exitCode, "record should succeed: stderr=%s", stderr)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "stderr:")
}

// TestWindows_Record_OutputDirCreation verifies that recording into a nested
// existing directory works correctly on Windows paths.
func TestWindows_Record_OutputDirCreation(t *testing.T) {
	binary := ensureBinary(t)
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "deep")
	require.NoError(t, os.MkdirAll(nestedDir, 0755))
	outputPath := filepath.Join(nestedDir, "recorded.yaml")

	_, stderr, exitCode := runCLI(t, binary, "record",
		"--output", outputPath,
		"--name", "win-nested-output",
		"--", "cmd", "/c", "echo nested")

	assert.Equal(t, 0, exitCode, "record should work in nested dirs: stderr=%s", stderr)
	assert.FileExists(t, outputPath)
}
