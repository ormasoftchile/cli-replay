package recorder

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionMetadata_Validate(t *testing.T) {
	tests := []struct {
		name    string
		meta    SessionMetadata
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid metadata",
			meta: SessionMetadata{
				Name:        "test-scenario",
				Description: "A test scenario",
				RecordedAt:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "empty name",
			meta: SessionMetadata{
				Name:        "",
				Description: "A test scenario",
				RecordedAt:  time.Now(),
			},
			wantErr: true,
			errMsg:  "name must be non-empty",
		},
		{
			name: "zero timestamp",
			meta: SessionMetadata{
				Name:        "test-scenario",
				Description: "A test scenario",
				RecordedAt:  time.Time{},
			},
			wantErr: true,
			errMsg:  "recordedAt must not be zero",
		},
		{
			name: "empty description is valid",
			meta: SessionMetadata{
				Name:        "test-scenario",
				Description: "",
				RecordedAt:  time.Now(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.meta.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRecordingSession_New(t *testing.T) {
	meta := SessionMetadata{
		Name:        "test-session",
		Description: "Test recording session",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	// Verify session fields
	assert.Equal(t, "test-session", session.Metadata.Name)
	assert.Equal(t, "Test recording session", session.Metadata.Description)
	assert.NotEmpty(t, session.ShimDir)
	assert.NotEmpty(t, session.LogFile)
	assert.Empty(t, session.Commands)

	// Verify temp directory was created
	assert.DirExists(t, session.ShimDir)
}

func TestRecordingSession_Cleanup(t *testing.T) {
	meta := SessionMetadata{
		Name:        "cleanup-test",
		Description: "Test cleanup",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)

	shimDir := session.ShimDir
	assert.DirExists(t, shimDir)

	err = session.Cleanup()
	require.NoError(t, err)

	// Verify temp directory was removed
	assert.NoDirExists(t, shimDir)
}

func TestRecordingSession_Finalize(t *testing.T) {
	meta := SessionMetadata{
		Name:        "finalize-test",
		Description: "Test finalization",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	// Create a simple JSONL log file
	logContent := `{"timestamp":"2024-01-15T10:30:00Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"NAME    READY\n","stderr":""}
{"timestamp":"2024-01-15T10:30:05Z","argv":["kubectl","describe","pod","pod1"],"exit":0,"stdout":"Name: pod1\n","stderr":""}
`
	err = os.WriteFile(session.LogFile, []byte(logContent), 0600)
	require.NoError(t, err)

	// Finalize the session
	err = session.Finalize()
	require.NoError(t, err)

	// Verify finalized state
	require.Len(t, session.Commands, 2)

	// Verify first command
	assert.Equal(t, []string{"kubectl", "get", "pods"}, session.Commands[0].Argv)
	assert.Equal(t, 0, session.Commands[0].ExitCode)
	assert.Equal(t, "NAME    READY\n", session.Commands[0].Stdout)

	// Verify second command
	assert.Equal(t, []string{"kubectl", "describe", "pod", "pod1"}, session.Commands[1].Argv)
	assert.Equal(t, 0, session.Commands[1].ExitCode)
	assert.Equal(t, "Name: pod1\n", session.Commands[1].Stdout)
}

func TestRecordingSession_Finalize_AlreadyFinalized(t *testing.T) {
	meta := SessionMetadata{
		Name:        "double-finalize-test",
		Description: "Test double finalization",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	// Create empty log
	err = os.WriteFile(session.LogFile, []byte(""), 0600)
	require.NoError(t, err)

	// First finalize
	err = session.Finalize()
	require.NoError(t, err)

	// Second finalize should succeed (idempotent)
	err = session.Finalize()
	assert.NoError(t, err)
}

func TestRecordingSession_Finalize_InvalidLog(t *testing.T) {
	meta := SessionMetadata{
		Name:        "invalid-log-test",
		Description: "Test invalid log handling",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	// Create invalid JSONL content
	err = os.WriteFile(session.LogFile, []byte("{invalid json}\n"), 0600)
	require.NoError(t, err)

	// Finalize should fail
	err = session.Finalize()
	assert.Error(t, err)
}

func TestRecordingSession_SetupShims_NoFilters(t *testing.T) {
	meta := SessionMetadata{
		Name:        "no-filters-test",
		Description: "Test shim setup with no filters",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	// SetupShims with no filters should be a no-op
	err = session.SetupShims()
	require.NoError(t, err)

	// Shim directory should exist but be empty (only the log file)
	entries, err := os.ReadDir(session.ShimDir)
	require.NoError(t, err)
	// Only the recording.jsonl file should exist
	assert.Len(t, entries, 1)
	assert.Equal(t, "recording.jsonl", entries[0].Name())
}

func TestRecordingSession_SetupShims_WithFilters(t *testing.T) {
	meta := SessionMetadata{
		Name:        "filtered-test",
		Description: "Test shim setup with filters",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{"kubectl", "docker"})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	err = session.SetupShims()
	require.NoError(t, err)

	// Verify shims were created for each filtered command
	kubectlShim := filepath.Join(session.ShimDir, "kubectl")
	dockerShim := filepath.Join(session.ShimDir, "docker")

	assert.FileExists(t, kubectlShim)
	assert.FileExists(t, dockerShim)

	// Verify shims are executable
	info, err := os.Stat(kubectlShim)
	require.NoError(t, err)
	assert.NotEqual(t, os.FileMode(0), info.Mode()&0111, "kubectl shim should be executable")

	// Verify shim content references the log file
	content, err := os.ReadFile(kubectlShim) //nolint:gosec // test file path
	require.NoError(t, err)
	assert.Contains(t, string(content), session.LogFile)
	assert.Contains(t, string(content), "#!/usr/bin/env bash")
}

func TestRecordingSession_Execute_DirectCapture(t *testing.T) {
	meta := SessionMetadata{
		Name:        "execute-test",
		Description: "Test direct execution capture",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	exitCode, err := session.Execute([]string{"echo", "hello world"}, &stdout, &stderr)
	require.NoError(t, err)

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello world\n", stdout.String())
	assert.Empty(t, stderr.String())

	// Verify command was recorded in session
	require.Len(t, session.Commands, 1)
	assert.Equal(t, []string{"echo", "hello world"}, session.Commands[0].Argv)
	assert.Equal(t, 0, session.Commands[0].ExitCode)
	assert.Equal(t, "hello world\n", session.Commands[0].Stdout)
	assert.NotZero(t, session.Commands[0].Timestamp)
}

func TestRecordingSession_Execute_NonZeroExit(t *testing.T) {
	meta := SessionMetadata{
		Name:        "nonzero-exit-test",
		Description: "Test non-zero exit capture",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	exitCode, err := session.Execute([]string{"sh", "-c", "echo fail >&2; exit 42"}, &stdout, &stderr)
	require.NoError(t, err)

	assert.Equal(t, 42, exitCode)
	assert.Contains(t, stderr.String(), "fail")

	require.Len(t, session.Commands, 1)
	assert.Equal(t, 42, session.Commands[0].ExitCode)
}

func TestRecordingSession_Execute_StderrCapture(t *testing.T) {
	meta := SessionMetadata{
		Name:        "stderr-test",
		Description: "Test stderr capture",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	exitCode, err := session.Execute([]string{"sh", "-c", "echo out; echo err >&2"}, &stdout, &stderr)
	require.NoError(t, err)

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "out\n", stdout.String())
	assert.Equal(t, "err\n", stderr.String())

	require.Len(t, session.Commands, 1)
	assert.Equal(t, "out\n", session.Commands[0].Stdout)
	assert.Equal(t, "err\n", session.Commands[0].Stderr)
}

func TestRecordingSession_Execute_CommandNotFound(t *testing.T) {
	meta := SessionMetadata{
		Name:        "notfound-test",
		Description: "Test command not found",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	exitCode, err := session.Execute([]string{"nonexistent-command-12345"}, &stdout, &stderr)

	require.Error(t, err)
	assert.Equal(t, 127, exitCode)
}

func TestRecordingSession_Execute_EmptyArgs(t *testing.T) {
	meta := SessionMetadata{
		Name:        "empty-args-test",
		Description: "Test empty args",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	_, err = session.Execute([]string{}, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no command specified")
}

func TestRecordingSession_Execute_WritesToJSONL(t *testing.T) {
	meta := SessionMetadata{
		Name:        "jsonl-write-test",
		Description: "Test JSONL log is written during execute",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	_, err = session.Execute([]string{"echo", "logged"}, &stdout, &stderr)
	require.NoError(t, err)

	// Verify JSONL log was written
	logContent, err := os.ReadFile(session.LogFile)
	require.NoError(t, err)
	assert.Contains(t, string(logContent), "logged")
	assert.Contains(t, string(logContent), `"echo"`)
}

func TestRecordingSession_New_DefaultName(t *testing.T) {
	// Test auto-generated name when name is empty
	meta := SessionMetadata{
		Name:        "",
		Description: "Auto-name test",
		RecordedAt:  time.Now(),
	}

	session, err := New(meta, []string{})
	require.NoError(t, err)
	defer session.Cleanup() //nolint:errcheck // test cleanup

	// Name should have been auto-generated
	assert.Contains(t, session.Metadata.Name, "recorded-session-")
}
