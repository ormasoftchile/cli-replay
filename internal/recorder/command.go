// Package recorder provides functionality for recording CLI command executions
// and generating replay scenario YAML files.
package recorder

import (
	"errors"
	"fmt"
	"time"
)

// RecordedCommand represents a single command execution captured during a recording session.
type RecordedCommand struct {
	Timestamp time.Time `json:"timestamp"`
	Argv      []string  `json:"argv"`
	ExitCode  int       `json:"exit"`
	Stdout    string    `json:"stdout"`
	Stderr    string    `json:"stderr"`
	Stdin     string    `json:"stdin,omitempty"`
}

// Validate checks that the RecordedCommand is valid.
func (r *RecordedCommand) Validate() error {
	if len(r.Argv) == 0 {
		return errors.New("argv must be non-empty")
	}
	if r.ExitCode < 0 || r.ExitCode > 255 {
		return fmt.Errorf("exit code must be in range 0-255, got %d", r.ExitCode)
	}
	if r.Timestamp.IsZero() {
		return errors.New("timestamp must not be zero")
	}
	return nil
}
