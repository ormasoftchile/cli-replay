package recorder

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// RecordingEntry represents a single entry in a JSONL log file.
// This is the internal representation used for JSON unmarshaling.
type RecordingEntry struct {
	Timestamp string   `json:"timestamp"`
	Argv      []string `json:"argv"`
	Exit      int      `json:"exit"`
	Stdout    string   `json:"stdout"`
	Stderr    string   `json:"stderr"`
	Stdin     string   `json:"stdin,omitempty"`
	Encoding  string   `json:"encoding,omitempty"` // "" = UTF-8 text, "base64" = raw bytes
}

// RecordingLog represents the JSONL log file structure for parsing recorded commands.
type RecordingLog struct {
	Entries  []RecordingEntry
	FilePath string
}

// ReadRecordingLog parses a JSONL file and returns a RecordingLog.
func ReadRecordingLog(filePath string) (*RecordingLog, error) {
	file, err := os.Open(filePath) //nolint:gosec // file path from caller
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close() //nolint:errcheck // read-only file close

	var entries []RecordingEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue // Skip empty lines
		}

		var entry RecordingEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("invalid JSON at line %d: %w", lineNum, err)
		}

		// Validate required fields
		if len(entry.Argv) == 0 {
			return nil, fmt.Errorf("line %d: argv is required", lineNum)
		}
		if entry.Timestamp == "" {
			return nil, fmt.Errorf("line %d: timestamp is required", lineNum)
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	return &RecordingLog{
		Entries:  entries,
		FilePath: filePath,
	}, nil
}

// ToRecordedCommands converts RecordingEntry slice to RecordedCommand slice.
// If an entry has Encoding "base64", stdout and stderr are decoded before conversion.
func (l *RecordingLog) ToRecordedCommands() ([]RecordedCommand, error) {
	commands := make([]RecordedCommand, 0, len(l.Entries))

	for i, entry := range l.Entries {
		timestamp, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("entry %d: invalid timestamp format: %w", i, err)
		}

		stdout := entry.Stdout
		stderr := entry.Stderr

		// Decode base64-encoded output (FR-015)
		if entry.Encoding == "base64" {
			outBytes, err := base64.StdEncoding.DecodeString(entry.Stdout)
			if err != nil {
				return nil, fmt.Errorf("entry %d: failed to decode base64 stdout: %w", i, err)
			}
			errBytes, err := base64.StdEncoding.DecodeString(entry.Stderr)
			if err != nil {
				return nil, fmt.Errorf("entry %d: failed to decode base64 stderr: %w", i, err)
			}
			stdout = string(outBytes)
			stderr = string(errBytes)
		}

		cmd := RecordedCommand{
			Timestamp: timestamp,
			Argv:      entry.Argv,
			ExitCode:  entry.Exit,
			Stdout:    stdout,
			Stderr:    stderr,
			Stdin:     entry.Stdin,
		}

		if err := cmd.Validate(); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}

		commands = append(commands, cmd)
	}

	return commands, nil
}
