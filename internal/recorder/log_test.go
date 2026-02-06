package recorder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadRecordingLog(t *testing.T) {
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
			require.NoError(t, os.WriteFile(logPath, []byte(tt.content), 0644))

			got, err := ReadRecordingLog(logPath)
			if tt.wantErr {
				assert.Error(t, err)
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
	assert.Equal(t, "", commands[0].Stderr)

	assert.Equal(t, []string{"kubectl", "describe", "pod", "pod1"}, commands[1].Argv)
	assert.Equal(t, 1, commands[1].ExitCode)
	assert.Equal(t, "", commands[1].Stdout)
	assert.Equal(t, "Error\n", commands[1].Stderr)
}
