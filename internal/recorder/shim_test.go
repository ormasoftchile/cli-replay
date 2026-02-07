package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogRecording(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.jsonl")

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	err := LogRecording(logPath, ts, []string{"kubectl", "get", "pods"}, 0, "NAME    READY\n", "")
	require.NoError(t, err)

	// Verify file was written
	assert.FileExists(t, logPath)

	content, err := os.ReadFile(logPath) //nolint:gosec
	require.NoError(t, err)

	var entry RecordingEntry
	err = json.Unmarshal(content, &entry)
	require.NoError(t, err)

	assert.Equal(t, "2024-01-15T10:30:00Z", entry.Timestamp)
	assert.Equal(t, []string{"kubectl", "get", "pods"}, entry.Argv)
	assert.Equal(t, 0, entry.Exit)
	assert.Equal(t, "NAME    READY\n", entry.Stdout)
	assert.Empty(t, entry.Stderr)
}

func TestLogRecording_AppendMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "multi.jsonl")

	ts1 := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	ts2 := time.Date(2024, 1, 15, 10, 30, 5, 0, time.UTC)

	err := LogRecording(logPath, ts1, []string{"kubectl", "get", "pods"}, 0, "out1\n", "")
	require.NoError(t, err)

	err = LogRecording(logPath, ts2, []string{"kubectl", "describe", "pod"}, 1, "", "err2\n")
	require.NoError(t, err)

	// Read and verify both entries via ReadRecordingLog
	log, err := ReadRecordingLog(logPath)
	require.NoError(t, err)
	require.Len(t, log.Entries, 2)

	assert.Equal(t, "kubectl", log.Entries[0].Argv[0])
	assert.Equal(t, 0, log.Entries[0].Exit)
	assert.Equal(t, "kubectl", log.Entries[1].Argv[0])
	assert.Equal(t, 1, log.Entries[1].Exit)
}

func TestLogRecording_NonZeroExit(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "exit.jsonl")

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	err := LogRecording(logPath, ts, []string{"false"}, 1, "", "")
	require.NoError(t, err)

	log, err := ReadRecordingLog(logPath)
	require.NoError(t, err)
	require.Len(t, log.Entries, 1)
	assert.Equal(t, 1, log.Entries[0].Exit)
}

func TestLogRecording_InvalidPath(t *testing.T) {
	err := LogRecording("/nonexistent/dir/log.jsonl", time.Now(), []string{"cmd"}, 0, "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open log file")
}
