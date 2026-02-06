package recorder

import (
	"bufio"
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
func (l *RecordingLog) ToRecordedCommands() ([]RecordedCommand, error) {
	commands := make([]RecordedCommand, 0, len(l.Entries))

	for i, entry := range l.Entries {
		timestamp, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("entry %d: invalid timestamp format: %w", i, err)
		}

		cmd := RecordedCommand{
			Timestamp: timestamp,
			Argv:      entry.Argv,
			ExitCode:  entry.Exit,
			Stdout:    entry.Stdout,
			Stderr:    entry.Stderr,
		}

		if err := cmd.Validate(); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}

		commands = append(commands, cmd)
	}

	return commands, nil
}
