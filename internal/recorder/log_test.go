package recorder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadRecordingLog(t *testing.T) { //nolint:funlen // table-driven test
	tests := []struct {
		name    string
		content string
		want    []RecordingEntry
		wantErr bool
		errMsg  string
	}{
		{
			name: "single entry",
			content: `{"timestamp":"2024-01-15T10:30:00Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"NAME    READY\n","stderr":""}
`,
			want: []RecordingEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Argv:      []string{"kubectl", "get", "pods"},
					Exit:      0,
					Stdout:    "NAME    READY\n",
					Stderr:    "",
				},
			},
			wantErr: false,
		},
		{
			name: "multiple entries",
			content: `{"timestamp":"2024-01-15T10:30:00Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"pod1\n","stderr":""}
{"timestamp":"2024-01-15T10:30:05Z","argv":["kubectl","describe","pod","pod1"],"exit":0,"stdout":"Name: pod1\n","stderr":""}
`,
			want: []RecordingEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Argv:      []string{"kubectl", "get", "pods"},
					Exit:      0,
					Stdout:    "pod1\n",
					Stderr:    "",
				},
				{
					Timestamp: "2024-01-15T10:30:05Z",
					Argv:      []string{"kubectl", "describe", "pod", "pod1"},
					Exit:      0,
					Stdout:    "Name: pod1\n",
					Stderr:    "",
				},
			},
			wantErr: false,
		},
		{
			name: "entry with stderr",
			content: `{"timestamp":"2024-01-15T10:30:00Z","argv":["kubectl","get","pod","nonexistent"],"exit":1,"stdout":"","stderr":"Error from server (NotFound)\n"}
`,
			want: []RecordingEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Argv:      []string{"kubectl", "get", "pod", "nonexistent"},
					Exit:      1,
					Stdout:    "",
					Stderr:    "Error from server (NotFound)\n",
				},
			},
			wantErr: false,
		},
		{
			name:    "empty file",
			content: "",
			want:    nil, // nil slice expected for empty file
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			content: `{invalid json}`,
			wantErr: true,
			errMsg:  "invalid JSON at line 1",
		},
		{
			name: "blank lines ignored",
			content: `{"timestamp":"2024-01-15T10:30:00Z","argv":["cmd"],"exit":0,"stdout":"","stderr":""}

{"timestamp":"2024-01-15T10:30:05Z","argv":["cmd2"],"exit":0,"stdout":"","stderr":""}
`,
			want: []RecordingEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Argv:      []string{"cmd"},
					Exit:      0,
					Stdout:    "",
					Stderr:    "",
				},
				{
					Timestamp: "2024-01-15T10:30:05Z",
					Argv:      []string{"cmd2"},
					Exit:      0,
					Stdout:    "",
					Stderr:    "",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file with test content
			tmpDir := t.TempDir()
			logPath := filepath.Join(tmpDir, "recording.log")
			require.NoError(t, os.WriteFile(logPath, []byte(tt.content), 0600))

			got, err := ReadRecordingLog(logPath)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got.Entries)
			}
		})
	}
}

func TestReadRecordingLog_FileNotFound(t *testing.T) {
	_, err := ReadRecordingLog("/nonexistent/path/recording.log")
	assert.Error(t, err)
}

// TestReadRecordingLog_MultiEntryOrderPreservation verifies that when parsing
// a JSONL file with many entries, the order is strictly preserved (T028).
func TestReadRecordingLog_MultiEntryOrderPreservation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "multi.jsonl")

	// 5 entries with sequential timestamps — order must be preserved
	content := `{"timestamp":"2024-01-15T10:00:01Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"step1\n","stderr":""}
{"timestamp":"2024-01-15T10:00:02Z","argv":["kubectl","delete","pod","web-0"],"exit":0,"stdout":"step2\n","stderr":""}
{"timestamp":"2024-01-15T10:00:03Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"step3\n","stderr":""}
{"timestamp":"2024-01-15T10:00:04Z","argv":["docker","build","-t","app:v1","."],"exit":0,"stdout":"step4\n","stderr":""}
{"timestamp":"2024-01-15T10:00:05Z","argv":["kubectl","apply","-f","deploy.yaml"],"exit":1,"stdout":"","stderr":"error\n"}
`
	require.NoError(t, os.WriteFile(logPath, []byte(content), 0600))

	log, err := ReadRecordingLog(logPath)
	require.NoError(t, err)
	require.Len(t, log.Entries, 5)

	// Verify strict order preservation
	expectedStdout := []string{"step1\n", "step2\n", "step3\n", "step4\n", ""}
	expectedArgv0 := []string{"kubectl", "kubectl", "kubectl", "docker", "kubectl"}

	for i, entry := range log.Entries {
		assert.Equal(t, expectedStdout[i], entry.Stdout, "entry %d stdout mismatch", i)
		assert.Equal(t, expectedArgv0[i], entry.Argv[0], "entry %d argv[0] mismatch", i)
	}

	// Verify last entry has non-zero exit and stderr
	assert.Equal(t, 1, log.Entries[4].Exit)
	assert.Equal(t, "error\n", log.Entries[4].Stderr)
}

// TestReadRecordingLog_DuplicateArgvPreserved verifies that entries with identical
// argv are all preserved as separate entries (FR-009b: no deduplication).
func TestReadRecordingLog_DuplicateArgvPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "dupes.jsonl")

	// Same command 3 times with different outputs
	content := `{"timestamp":"2024-01-15T10:00:01Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"before\n","stderr":""}
{"timestamp":"2024-01-15T10:00:02Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"during\n","stderr":""}
{"timestamp":"2024-01-15T10:00:03Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"after\n","stderr":""}
`
	require.NoError(t, os.WriteFile(logPath, []byte(content), 0600))

	log, err := ReadRecordingLog(logPath)
	require.NoError(t, err)
	require.Len(t, log.Entries, 3, "all 3 duplicate entries must be preserved")

	// All have same argv
	for i, entry := range log.Entries {
		assert.Equal(t, []string{"kubectl", "get", "pods"}, entry.Argv, "entry %d", i)
	}

	// But different stdout
	assert.Equal(t, "before\n", log.Entries[0].Stdout)
	assert.Equal(t, "during\n", log.Entries[1].Stdout)
	assert.Equal(t, "after\n", log.Entries[2].Stdout)
}

// TestReadRecordingLog_MissingRequiredFields verifies proper error on missing fields.
func TestReadRecordingLog_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		content string
		errMsg  string
	}{
		{
			name:    "missing argv",
			content: `{"timestamp":"2024-01-15T10:00:00Z","exit":0,"stdout":"","stderr":""}`,
			errMsg:  "argv is required",
		},
		{
			name:    "missing timestamp",
			content: `{"argv":["cmd"],"exit":0,"stdout":"","stderr":""}`,
			errMsg:  "timestamp is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			logPath := filepath.Join(tmpDir, "test.jsonl")
			require.NoError(t, os.WriteFile(logPath, []byte(tt.content+"\n"), 0600))

			_, err := ReadRecordingLog(logPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestRecordingLog_ToRecordedCommands_OrderPreservation converts a multi-entry log
// and verifies the order is maintained in the resulting command slice.
func TestRecordingLog_ToRecordedCommands_OrderPreservation(t *testing.T) {
	log := &RecordingLog{
		Entries: []RecordingEntry{
			{Timestamp: "2024-01-15T10:00:01Z", Argv: []string{"cmd1"}, Exit: 0, Stdout: "a", Stderr: ""},
			{Timestamp: "2024-01-15T10:00:02Z", Argv: []string{"cmd2"}, Exit: 0, Stdout: "b", Stderr: ""},
			{Timestamp: "2024-01-15T10:00:03Z", Argv: []string{"cmd3"}, Exit: 0, Stdout: "c", Stderr: ""},
			{Timestamp: "2024-01-15T10:00:04Z", Argv: []string{"cmd1"}, Exit: 1, Stdout: "d", Stderr: "e"},
			{Timestamp: "2024-01-15T10:00:05Z", Argv: []string{"cmd4"}, Exit: 0, Stdout: "f", Stderr: ""},
		},
	}

	commands, err := log.ToRecordedCommands()
	require.NoError(t, err)
	require.Len(t, commands, 5)

	// Verify ordering by argv
	expectedArgv := []string{"cmd1", "cmd2", "cmd3", "cmd1", "cmd4"}
	for i, cmd := range commands {
		assert.Equal(t, expectedArgv[i], cmd.Argv[0], "command %d", i)
	}

	// Verify timestamps are parsed correctly and in order
	for i := 1; i < len(commands); i++ {
		assert.True(t, commands[i].Timestamp.After(commands[i-1].Timestamp),
			"command %d timestamp should be after command %d", i, i-1)
	}
}

func TestRecordingLog_ToRecordedCommands(t *testing.T) {
	log := &RecordingLog{
		Entries: []RecordingEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Argv:      []string{"kubectl", "get", "pods"},
				Exit:      0,
				Stdout:    "pod1\n",
				Stderr:    "",
			},
			{
				Timestamp: "2024-01-15T10:30:05Z",
				Argv:      []string{"kubectl", "describe", "pod", "pod1"},
				Exit:      1,
				Stdout:    "",
				Stderr:    "Error\n",
			},
		},
	}

	commands, err := log.ToRecordedCommands()
	require.NoError(t, err)
	require.Len(t, commands, 2)

	assert.Equal(t, []string{"kubectl", "get", "pods"}, commands[0].Argv)
	assert.Equal(t, 0, commands[0].ExitCode)
	assert.Equal(t, "pod1\n", commands[0].Stdout)
	assert.Empty(t, commands[0].Stderr)

	assert.Equal(t, []string{"kubectl", "describe", "pod", "pod1"}, commands[1].Argv)
	assert.Equal(t, 1, commands[1].ExitCode)
	assert.Empty(t, commands[1].Stdout)
	assert.Equal(t, "Error\n", commands[1].Stderr)
}
