package recorder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateShim(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		logPath     string
		shimDir     string
		wantContent []string
		wantErr     bool
	}{
		{
			name:    "simple command",
			command: "kubectl",
			logPath: "/tmp/recording.jsonl",
			shimDir: "/tmp/shims",
			wantContent: []string{
				"#!/usr/bin/env bash",
				"LOGFILE=\"/tmp/recording.jsonl\"",
				"CLI_REPLAY_IN_SHIM",
				"kubectl",
				">>",
			},
			wantErr: false,
		},
		{
			name:    "hyphenated command",
			command: "docker-compose",
			logPath: "/var/tmp/logs/session.jsonl",
			shimDir: "/var/tmp/shims",
			wantContent: []string{
				"#!/usr/bin/env bash",
				"LOGFILE=\"/var/tmp/logs/session.jsonl\"",
				"CLI_REPLAY_IN_SHIM",
				"docker-compose",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := GenerateShim(tt.command, tt.logPath, tt.shimDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, content)
				
				// Verify content contains expected strings
				for _, expected := range tt.wantContent {
					assert.Contains(t, content, expected, "shim should contain %q", expected)
				}
			}
		})
	}
}

func TestWriteShim(t *testing.T) {
	tmpDir := t.TempDir()
	shimDir := filepath.Join(tmpDir, "shims")
	shimPath := filepath.Join(shimDir, "kubectl")
	logPath := filepath.Join(tmpDir, "recording.jsonl")

	err := WriteShim(shimPath, "kubectl", logPath, shimDir)
	require.NoError(t, err)

	// Verify shim file was created
	assert.FileExists(t, shimPath)

	// Verify shim is executable
	info, err := os.Stat(shimPath)
	require.NoError(t, err)
	
	// Check executable bit (mode should have x permission)
	mode := info.Mode()
	assert.True(t, mode&0111 != 0, "shim should be executable")

	// Verify content
	content, err := os.ReadFile(shimPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "#!/usr/bin/env bash")
	assert.Contains(t, string(content), logPath)
	assert.Contains(t, string(content), "CLI_REPLAY_IN_SHIM")
}

func TestGenerateAllShims(t *testing.T) {
	tmpDir := t.TempDir()
	shimDir := filepath.Join(tmpDir, "shims")
	logPath := filepath.Join(tmpDir, "recording.jsonl")

	commands := []string{"kubectl", "docker", "git"}
	
	err := GenerateAllShims(shimDir, commands, logPath)
	require.NoError(t, err)

	// Verify shim directory was created
	assert.DirExists(t, shimDir)

	// Verify all shims were created
	for _, cmd := range commands {
		shimPath := filepath.Join(shimDir, cmd)
		assert.FileExists(t, shimPath)

		// Verify executable
		info, err := os.Stat(shimPath)
		require.NoError(t, err)
		assert.True(t, info.Mode()&0111 != 0, "%s should be executable", cmd)
	}
}

func TestGenerateAllShims_EmptyList(t *testing.T) {
	tmpDir := t.TempDir()
	shimDir := filepath.Join(tmpDir, "shims")
	logPath := filepath.Join(tmpDir, "recording.jsonl")

	err := GenerateAllShims(shimDir, []string{}, logPath)
	assert.NoError(t, err)

	// Shim directory should still be created
	assert.DirExists(t, shimDir)
}

func TestWriteShim_InvalidPath(t *testing.T) {
	// Try to write to non-existent directory without creation
	err := WriteShim("/nonexistent/path/kubectl", "kubectl", "/tmp/log.jsonl", "/tmp/shims")
	assert.Error(t, err)
}
