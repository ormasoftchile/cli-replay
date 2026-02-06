package recorder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionMetadata contains user-provided metadata for the generated scenario.
type SessionMetadata struct {
	Name        string
	Description string
	RecordedAt  time.Time
}

// Validate checks that the SessionMetadata is valid.
func (m *SessionMetadata) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name must be non-empty")
	}
	if m.RecordedAt.IsZero() {
		return errors.New("recordedAt must not be zero")
	}
	return nil
}

// RecordingSession manages the lifecycle of a recording session.
type RecordingSession struct {
	ID       string
	StartTime time.Time
	EndTime   time.Time
	Commands  []RecordedCommand
	Filters   []string
	ShimDir   string
	LogFile   string
	Metadata  SessionMetadata
}

// New creates a new RecordingSession with the given metadata and filters.
func New(metadata SessionMetadata, filters []string) (*RecordingSession, error) {
	// Set defaults for metadata
	if metadata.Name == "" {
		metadata.Name = fmt.Sprintf("recorded-session-%s", time.Now().UTC().Format("20060102-150405"))
	}
	if metadata.RecordedAt.IsZero() {
		metadata.RecordedAt = time.Now().UTC()
	}

	if err := metadata.Validate(); err != nil {
		return nil, fmt.Errorf("invalid metadata: %w", err)
	}

	// Create temporary directory for shims
	shimDir, err := os.MkdirTemp("", "cli-replay-shims-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create shim directory: %w", err)
	}

	// Create log file in temp directory
	logFile := filepath.Join(shimDir, "recording.jsonl")

	// Create empty log file
	if err := os.WriteFile(logFile, []byte(""), 0644); err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	session := &RecordingSession{
		ID:        fmt.Sprintf("session-%d", time.Now().UnixNano()),
		StartTime: time.Now().UTC(),
		Commands:  []RecordedCommand{},
		Filters:   filters,
		ShimDir:   shimDir,
		LogFile:   logFile,
		Metadata:  metadata,
	}

	return session, nil
}

// Finalize marks the session as complete and performs cleanup.
func (s *RecordingSession) Finalize() error {
	s.EndTime = time.Now().UTC()

	// Parse the JSONL log to populate Commands
	log, err := ReadRecordingLog(s.LogFile)
	if err != nil {
		// If log file doesn't exist or is empty, that's okay (no commands recorded)
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read recording log: %w", err)
	}

	commands, err := log.ToRecordedCommands()
	if err != nil {
		return fmt.Errorf("failed to parse recorded commands: %w", err)
	}

	s.Commands = commands
	return nil
}

// Cleanup removes the temporary shim directory and all its contents.
func (s *RecordingSession) Cleanup() error {
	if s.ShimDir != "" {
		if err := os.RemoveAll(s.ShimDir); err != nil {
			return fmt.Errorf("failed to cleanup shim directory: %w", err)
		}
	}
	return nil
}
