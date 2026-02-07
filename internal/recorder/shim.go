package recorder

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unicode/utf8"
)

// LogRecording appends a command execution entry to the JSONL log file.
// This is used by shim scripts to record command executions.
// If stdout or stderr contain non-UTF-8 bytes, the content is base64-encoded
// and the Encoding field is set to "base64" (FR-015).
// stdin is included in the entry when non-empty (captured from piped input).
func LogRecording(logPath string, timestamp time.Time, argv []string, exitCode int, stdout, stderr, stdin string) error {
	entry := RecordingEntry{
		Timestamp: timestamp.Format(time.RFC3339),
		Argv:      argv,
		Exit:      exitCode,
		Stdout:    stdout,
		Stderr:    stderr,
		Stdin:     stdin,
	}

	// If either stdout or stderr contains non-UTF-8 bytes, base64-encode both
	if !utf8.ValidString(stdout) || !utf8.ValidString(stderr) {
		entry.Stdout = base64.StdEncoding.EncodeToString([]byte(stdout))
		entry.Stderr = base64.StdEncoding.EncodeToString([]byte(stderr))
		entry.Encoding = "base64"
	}

	// Open log file in append mode
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // log file needs to be readable
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close() //nolint:errcheck // best-effort close

	// Write JSONL entry
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(entry); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}
