package recorder

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	ID        string
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
	if err := os.WriteFile(logFile, []byte(""), 0600); err != nil {
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

// SetupShims generates shim scripts for the session's filtered commands.
// If no filters are specified, shims are not generated (direct capture is used instead).
// The generated shims are placed in the session's ShimDir.
func (s *RecordingSession) SetupShims() error {
	if len(s.Filters) == 0 {
		// No filters → direct capture mode (no shims needed)
		return nil
	}

	return GenerateAllShims(s.ShimDir, s.Filters, s.LogFile)
}

// Execute runs a command while recording its execution.
// In direct-capture mode (no command filters), the command's stdout and stderr
// are captured directly and a single RecordedCommand is produced.
// In shim mode (with command filters), shims intercept commands via PATH and
// log executions to the JSONL file.
func (s *RecordingSession) Execute(args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("no command specified")
	}

	if len(s.Filters) > 0 {
		// Shim mode: run command with shims prepended to PATH
		return s.executeWithShims(args, stdout, stderr)
	}

	// Direct capture mode: run command and capture output
	return s.executeAndCapture(args, stdout, stderr)
}

// executeWithShims runs the command in a subprocess with the shim directory
// prepended to PATH so that intercepted commands are logged to JSONL.
func (s *RecordingSession) executeWithShims(args []string, stdout, stderr io.Writer) (int, error) {
	cmdStr := strings.Join(args, " ")
	command := exec.Command("bash", "-c", cmdStr) //nolint:gosec,noctx // user command is intentionally executed

	// Modify PATH to include shim directory first
	originalPath := os.Getenv("PATH")
	modifiedPath := s.ShimDir + string(os.PathListSeparator) + originalPath

	// Build environment with modified PATH and recording variables
	env := os.Environ()
	newEnv := make([]string, 0, len(env)+3)
	for _, e := range env {
		if !strings.HasPrefix(e, "PATH=") {
			newEnv = append(newEnv, e)
		}
	}
	newEnv = append(newEnv,
		"PATH="+modifiedPath,
		"CLI_REPLAY_RECORDING_LOG="+s.LogFile,
		"CLI_REPLAY_SHIM_DIR="+s.ShimDir,
	)
	command.Env = newEnv

	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = os.Stdin

	err := command.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return 127, fmt.Errorf("command execution failed: %w", err)
		}
	}

	return exitCode, nil
}

// executeAndCapture runs a command directly and captures its stdout/stderr
// both for recording and for passing through to the caller's writers.
func (s *RecordingSession) executeAndCapture(args []string, stdout, stderr io.Writer) (int, error) {
	command := exec.Command(args[0], args[1:]...) //nolint:gosec,noctx // user command is intentionally executed

	// Capture stdout and stderr while also writing to callers
	var outBuf, errBuf strings.Builder
	command.Stdout = io.MultiWriter(stdout, &outBuf)
	command.Stderr = io.MultiWriter(stderr, &errBuf)
	command.Stdin = os.Stdin

	runErr := command.Run()

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return 127, fmt.Errorf("command execution failed: %w", runErr)
		}
	}

	// Create the recorded command
	recorded := RecordedCommand{
		Timestamp: time.Now().UTC(),
		Argv:      args,
		ExitCode:  exitCode,
		Stdout:    outBuf.String(),
		Stderr:    errBuf.String(),
	}

	s.Commands = append(s.Commands, recorded)

	// Also write to JSONL log for consistency
	if err := LogRecording(s.LogFile, recorded.Timestamp, recorded.Argv, recorded.ExitCode, recorded.Stdout, recorded.Stderr); err != nil {
		return exitCode, fmt.Errorf("failed to write recording log: %w", err)
	}

	return exitCode, nil
}
