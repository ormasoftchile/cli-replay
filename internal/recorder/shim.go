package recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// shimTemplate is the bash script template that intercepts command executions.
const shimTemplate = `#!/usr/bin/env bash
# cli-replay shim for: %s
# This script intercepts the command and logs execution details to JSONL

# Prevent recursive shim execution
if [ "$CLI_REPLAY_IN_SHIM" = "1" ]; then
    exec %s "$@"
fi
export CLI_REPLAY_IN_SHIM=1

LOGFILE="%s"

# Find real command by removing shim directory from PATH
REAL_COMMAND=$(PATH="${PATH#%s:}" command -v %s 2>/dev/null)

# If real command not found, report error
if [ -z "$REAL_COMMAND" ] || [ "$REAL_COMMAND" = "${BASH_SOURCE[0]}" ]; then
    echo "cli-replay: command not found: %s" >&2
    exit 127
fi

# Capture start time (RFC3339 format)
TIMESTAMP=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)

# Execute the real command and capture output
STDOUT_FILE=$(mktemp)
STDERR_FILE=$(mktemp)
EXIT_CODE=0

"$REAL_COMMAND" "$@" >"$STDOUT_FILE" 2>"$STDERR_FILE" || EXIT_CODE=$?

# Read captured output
STDOUT_CONTENT=$(cat "$STDOUT_FILE")
STDERR_CONTENT=$(cat "$STDERR_FILE")

# Echo output to preserve command behavior
cat "$STDOUT_FILE"
cat "$STDERR_FILE" >&2

# Clean up temp files
rm -f "$STDOUT_FILE" "$STDERR_FILE"

# Build argv array for JSON
ARGV_JSON="[\"%s\""
for arg in "$@"; do
    # Escape quotes and backslashes
    ESCAPED=$(printf '%%s' "$arg" | sed 's/\\/\\\\/g; s/"/\\"/g')
    ARGV_JSON="$ARGV_JSON,\"$ESCAPED\""
done
ARGV_JSON="$ARGV_JSON]"

# Escape JSON strings
ESC_STDOUT=$(printf '%%s' "$STDOUT_CONTENT" | sed 's/\\/\\\\/g; s/"/\\"/g' | awk '{printf "%%s\\n", $0}' | sed 's/\\n$//')
ESC_STDERR=$(printf '%%s' "$STDERR_CONTENT" | sed 's/\\/\\\\/g; s/"/\\"/g' | awk '{printf "%%s\\n", $0}' | sed 's/\\n$//')

# Write JSONL entry
printf '{"timestamp":"%%s","argv":%%s,"exit":%%d,"stdout":"%%s","stderr":"%%s"}\n' \
    "$TIMESTAMP" "$ARGV_JSON" "$EXIT_CODE" "$ESC_STDOUT" "$ESC_STDERR" >> "$LOGFILE"

exit $EXIT_CODE
`

// GenerateShim creates a bash shim script that intercepts command execution.
func GenerateShim(command string, logPath string, shimDir string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command must be non-empty")
	}
	if logPath == "" {
		return "", fmt.Errorf("logPath must be non-empty")
	}
	if shimDir == "" {
		return "", fmt.Errorf("shimDir must be non-empty")
	}

	// Format the template with command name, log path, and shim directory
	script := fmt.Sprintf(shimTemplate,
		command,   // Comment line
		command,   // exec fallback for recursive calls
		logPath,   // LOGFILE variable
		shimDir,   // PATH prefix to remove
		command,   // command -v lookup
		command,   // Error message
		command,   // argv[0] in JSON
	)

	return script, nil
}

// WriteShim writes a shim script to the specified path and makes it executable.
func WriteShim(shimPath string, command string, logPath string, shimDir string) error {
	content, err := GenerateShim(command, logPath, shimDir)
	if err != nil {
		return fmt.Errorf("failed to generate shim: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(shimPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create shim directory: %w", err)
	}

	// Write shim file with executable permissions
	if err := os.WriteFile(shimPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("failed to write shim file: %w", err)
	}

	return nil
}

// GenerateAllShims creates shim scripts for all specified commands.
func GenerateAllShims(shimDir string, commands []string, logPath string) error {
	// Create shim directory if it doesn't exist
	if err := os.MkdirAll(shimDir, 0755); err != nil {
		return fmt.Errorf("failed to create shim directory: %w", err)
	}

	// Generate shim for each command
	for _, cmd := range commands {
		shimPath := filepath.Join(shimDir, cmd)
		if err := WriteShim(shimPath, cmd, logPath, shimDir); err != nil {
			return fmt.Errorf("failed to write shim for %s: %w", cmd, err)
		}
	}

	return nil
}

// LogRecording appends a command execution entry to the JSONL log file.
// This is used by shim scripts to record command executions.
func LogRecording(logPath string, timestamp time.Time, argv []string, exitCode int, stdout, stderr string) error {
	entry := RecordingEntry{
		Timestamp: timestamp.Format(time.RFC3339),
		Argv:      argv,
		Exit:      exitCode,
		Stdout:    stdout,
		Stderr:    stderr,
	}

	// Open log file in append mode
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Write JSONL entry
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(entry); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}

// FindRealCommand locates the actual binary for a command, excluding shims.
func FindRealCommand(command string, shimDir string) (string, error) {
	// Use 'command -v' to find the command in PATH
	cmd := exec.Command("sh", "-c", fmt.Sprintf("command -v %s", command))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("command not found: %s", command)
	}

	realPath := string(output)
	// Check if it's not our shim
	if filepath.Dir(realPath) == shimDir {
		// If it's in our shim dir, look for the real one in common locations
		commonPaths := []string{
			"/usr/bin/" + command,
			"/usr/local/bin/" + command,
			"/bin/" + command,
		}
		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
		return "", fmt.Errorf("real command not found for: %s", command)
	}

	return realPath, nil
}
