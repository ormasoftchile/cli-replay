package recorder

import (
	"os"
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
				assert.Error(t, err)
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
	defer session.Cleanup()

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
	assert.NoError(t, err)

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
	defer session.Cleanup()

	// Create a simple JSONL log file
	logContent := `{"timestamp":"2024-01-15T10:30:00Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"NAME    READY\n","stderr":""}
{"timestamp":"2024-01-15T10:30:05Z","argv":["kubectl","describe","pod","pod1"],"exit":0,"stdout":"Name: pod1\n","stderr":""}
`
	err = os.WriteFile(session.LogFile, []byte(logContent), 0644)
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
	defer session.Cleanup()

	// Create empty log
	err = os.WriteFile(session.LogFile, []byte(""), 0644)
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
	defer session.Cleanup()

	// Create invalid JSONL content
	err = os.WriteFile(session.LogFile, []byte("{invalid json}\n"), 0644)
	require.NoError(t, err)

	// Finalize should fail
	err = session.Finalize()
	assert.Error(t, err)
}
