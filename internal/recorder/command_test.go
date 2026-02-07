package recorder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordedCommand_Validate(t *testing.T) { //nolint:funlen // table-driven test
	tests := []struct {
		name    string
		cmd     RecordedCommand
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid command",
			cmd: RecordedCommand{
				Timestamp: time.Now(),
				Argv:      []string{"kubectl", "get", "pods"},
				ExitCode:  0,
				Stdout:    "NAME    READY   STATUS\n",
				Stderr:    "",
			},
			wantErr: false,
		},
		{
			name: "empty argv",
			cmd: RecordedCommand{
				Timestamp: time.Now(),
				Argv:      []string{},
				ExitCode:  0,
			},
			wantErr: true,
			errMsg:  "argv must be non-empty",
		},
		{
			name: "exit code too low",
			cmd: RecordedCommand{
				Timestamp: time.Now(),
				Argv:      []string{"cmd"},
				ExitCode:  -1,
			},
			wantErr: true,
			errMsg:  "exit code must be in range 0-255",
		},
		{
			name: "exit code too high",
			cmd: RecordedCommand{
				Timestamp: time.Now(),
				Argv:      []string{"cmd"},
				ExitCode:  256,
			},
			wantErr: true,
			errMsg:  "exit code must be in range 0-255",
		},
		{
			name: "zero timestamp",
			cmd: RecordedCommand{
				Timestamp: time.Time{},
				Argv:      []string{"cmd"},
				ExitCode:  0,
			},
			wantErr: true,
			errMsg:  "timestamp must not be zero",
		},
		{
			name: "empty stdout and stderr is valid",
			cmd: RecordedCommand{
				Timestamp: time.Now(),
				Argv:      []string{"cmd"},
				ExitCode:  0,
				Stdout:    "",
				Stderr:    "",
			},
			wantErr: false,
		},
		{
			name: "non-zero exit code with stderr",
			cmd: RecordedCommand{
				Timestamp: time.Now(),
				Argv:      []string{"cmd", "--invalid-flag"},
				ExitCode:  1,
				Stdout:    "",
				Stderr:    "Error: unknown flag: --invalid-flag\n",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
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
