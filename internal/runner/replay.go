package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cli-replay/cli-replay/internal/matcher"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/cli-replay/cli-replay/internal/template"
)

// ReplayResult contains the outcome of a replay operation.
type ReplayResult struct {
	ExitCode     int
	Matched      bool
	StepIndex    int
	ScenarioName string
}

// ReplayResponse writes the step's response to stdout/stderr and returns the exit code.
// This variant handles inline stdout/stderr only (no templates).
func ReplayResponse(step *scenario.Step, _ string, stdout, stderr io.Writer) int {
	if step.Respond.Stdout != "" {
		_, _ = io.WriteString(stdout, step.Respond.Stdout)
	}
	if step.Respond.Stderr != "" {
		_, _ = io.WriteString(stderr, step.Respond.Stderr)
	}
	return step.Respond.Exit
}

// ReplayResponseWithFile writes the step's response to stdout/stderr and returns the exit code.
// This variant handles both inline and file-based stdout/stderr (no templates).
func ReplayResponseWithFile(step *scenario.Step, scenarioPath string, stdout, stderr io.Writer) int {
	scenarioDir := filepath.Dir(scenarioPath)

	// Handle stdout
	if step.Respond.StdoutFile != "" {
		content, err := readFile(scenarioDir, step.Respond.StdoutFile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "cli-replay: failed to read stdout_file: %v\n", err)
			return 1
		}
		_, _ = io.WriteString(stdout, content)
	} else if step.Respond.Stdout != "" {
		_, _ = io.WriteString(stdout, step.Respond.Stdout)
	}

	// Handle stderr
	if step.Respond.StderrFile != "" {
		content, err := readFile(scenarioDir, step.Respond.StderrFile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "cli-replay: failed to read stderr_file: %v\n", err)
			return 1
		}
		_, _ = io.WriteString(stderr, content)
	} else if step.Respond.Stderr != "" {
		_, _ = io.WriteString(stderr, step.Respond.Stderr)
	}

	return step.Respond.Exit
}

// ReplayResponseWithTemplate writes the step's response with template rendering.
// Templates in stdout/stderr are rendered with vars from scenario meta + environment.
func ReplayResponseWithTemplate(step *scenario.Step, scn *scenario.Scenario, scenarioPath string, stdout, stderr io.Writer) int {
	scenarioDir := filepath.Dir(scenarioPath)
	vars := template.MergeVars(scn.Meta.Vars)

	// Handle stdout
	stdoutContent := ""
	if step.Respond.StdoutFile != "" {
		content, err := readFile(scenarioDir, step.Respond.StdoutFile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "cli-replay: failed to read stdout_file: %v\n", err)
			return 1
		}
		stdoutContent = content
	} else {
		stdoutContent = step.Respond.Stdout
	}

	if stdoutContent != "" {
		rendered, err := template.Render(stdoutContent, vars)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "cli-replay: failed to render stdout template: %v\n", err)
			return 1
		}
		_, _ = io.WriteString(stdout, rendered)
	}

	// Handle stderr
	stderrContent := ""
	if step.Respond.StderrFile != "" {
		content, err := readFile(scenarioDir, step.Respond.StderrFile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "cli-replay: failed to read stderr_file: %v\n", err)
			return 1
		}
		stderrContent = content
	} else {
		stderrContent = step.Respond.Stderr
	}

	if stderrContent != "" {
		rendered, err := template.Render(stderrContent, vars)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "cli-replay: failed to render stderr template: %v\n", err)
			return 1
		}
		_, _ = io.WriteString(stderr, rendered)
	}

	return step.Respond.Exit
}

// readFile reads a file relative to the base directory.
func readFile(baseDir, relPath string) (string, error) {
	fullPath := filepath.Join(baseDir, relPath)
	data, err := os.ReadFile(fullPath) //nolint:gosec // File path is relative to scenario directory
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ExecuteReplay runs the replay logic for a given scenario and argv.
// It loads the scenario, checks/creates state, matches the command, and returns the response.
//
//nolint:funlen // Complex function with many validation steps
func ExecuteReplay(scenarioPath string, argv []string, stdout, stderr io.Writer) (*ReplayResult, error) {
	// Load scenario
	absPath, err := filepath.Abs(scenarioPath)
	if err != nil {
		return &ReplayResult{ExitCode: 1}, fmt.Errorf("failed to resolve scenario path: %w", err)
	}

	scn, err := scenario.LoadFile(absPath)
	if err != nil {
		return &ReplayResult{ExitCode: 1}, fmt.Errorf("failed to load scenario: %w", err)
	}

	// Calculate scenario hash
	scenarioHash := hashScenarioFile(absPath)

	// Load or initialize state
	stateFile := StateFilePath(absPath)
	state, err := ReadState(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Initialize new state
			state = NewState(absPath, scenarioHash, len(scn.Steps))
		} else {
			return &ReplayResult{ExitCode: 1}, fmt.Errorf("failed to read state: %w", err)
		}
	}

	// Check if scenario completed
	if state.IsComplete() {
		_, _ = fmt.Fprintf(stderr, "cli-replay: scenario %q already complete (all %d steps consumed)\n",
			scn.Meta.Name, state.TotalSteps)
		return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name},
			fmt.Errorf("scenario already complete")
	}

	// Get expected step
	stepIndex := state.CurrentStep
	if stepIndex >= len(scn.Steps) {
		return &ReplayResult{ExitCode: 1}, fmt.Errorf("step index out of range")
	}
	expectedStep := &scn.Steps[stepIndex]

	// Match argv
	if !matcher.ArgvMatch(expectedStep.Match.Argv, argv) {
		result := &ReplayResult{
			ExitCode:     1,
			Matched:      false,
			StepIndex:    stepIndex,
			ScenarioName: scn.Meta.Name,
		}
		return result, &MismatchError{
			Scenario:  scn.Meta.Name,
			StepIndex: stepIndex,
			Expected:  expectedStep.Match.Argv,
			Received:  argv,
		}
	}

	// Execute response with template rendering
	exitCode := ReplayResponseWithTemplate(expectedStep, scn, absPath, stdout, stderr)

	// Trace output if enabled
	if IsTraceEnabled(os.Getenv(TraceEnvVar)) {
		WriteTraceOutput(stderr, stepIndex, argv, exitCode)
	}

	// Advance state
	state.Advance()
	if err := WriteState(stateFile, state); err != nil {
		_, _ = fmt.Fprintf(stderr, "cli-replay: warning: failed to save state: %v\n", err)
	}

	return &ReplayResult{
		ExitCode:     exitCode,
		Matched:      true,
		StepIndex:    stepIndex,
		ScenarioName: scn.Meta.Name,
	}, nil
}

// hashScenarioFile calculates SHA256 hash of the scenario file content.
func hashScenarioFile(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // File path from user input
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// MismatchError represents an argv mismatch during replay.
type MismatchError struct {
	Scenario  string
	StepIndex int
	Expected  []string
	Received  []string
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("argv mismatch at step %d", e.StepIndex)
}
