package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

	// Budget-aware step matching
	// Skip exhausted steps (count >= max), then try matching current step.
	// If current step doesn't match but its min is met, soft-advance to next step.
	stepIndex := state.CurrentStep

	// Phase 1: Skip exhausted steps
	for stepIndex < len(scn.Steps) {
		bounds := scn.Steps[stepIndex].EffectiveCalls()
		if state.StepBudgetRemaining(stepIndex, bounds.Max) > 0 {
			break
		}
		stepIndex++
	}

	// Update CurrentStep if we skipped exhausted steps
	if stepIndex > state.CurrentStep {
		state.CurrentStep = stepIndex
	}

	if stepIndex >= len(scn.Steps) {
		_, _ = fmt.Fprintf(stderr, "cli-replay: scenario %q already complete (all %d steps consumed)\n",
			scn.Meta.Name, state.TotalSteps)
		return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name},
			fmt.Errorf("scenario already complete")
	}

	expectedStep := &scn.Steps[stepIndex]

	// Phase 2: Try matching current step
	matched := matcher.ArgvMatch(expectedStep.Match.Argv, argv)

	// Phase 3: Soft-advance if current step doesn't match but min is met
	softAdvanced := false
	origStepIndex := stepIndex
	if !matched {
		bounds := expectedStep.EffectiveCalls()
		if state.StepCounts != nil && stepIndex < len(state.StepCounts) &&
			state.StepCounts[stepIndex] >= bounds.Min && stepIndex+1 < len(scn.Steps) {
			// Min satisfied — try next step
			softAdvanced = true
			stepIndex++
			state.CurrentStep = stepIndex
			expectedStep = &scn.Steps[stepIndex]
			matched = matcher.ArgvMatch(expectedStep.Match.Argv, argv)
		}
	}

	if !matched {
		result := &ReplayResult{
			ExitCode:     1,
			Matched:      false,
			StepIndex:    stepIndex,
			ScenarioName: scn.Meta.Name,
		}
		mErr := &MismatchError{
			Scenario:  scn.Meta.Name,
			StepIndex: stepIndex,
			Expected:  expectedStep.Match.Argv,
			Received:  argv,
		}
		if softAdvanced {
			mErr.SoftAdvanced = true
			mErr.NextStepIndex = stepIndex
			mErr.NextExpected = expectedStep.Match.Argv
			// Report against the original step
			mErr.StepIndex = origStepIndex
			mErr.Expected = scn.Steps[origStepIndex].Match.Argv
		}
		return result, mErr
	}

	// Increment call count for the matched step
	state.IncrementStep(stepIndex)

	// stdin matching: if the step defines match.stdin, read actual stdin and compare
	if expectedStep.Match.Stdin != "" {
		actualStdin, readErr := readStdin()
		if readErr != nil {
			_, _ = fmt.Fprintf(stderr, "cli-replay: failed to read stdin: %v\n", readErr)
			return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name}, readErr
		}
		if normalizeStdin(actualStdin) != normalizeStdin(expectedStep.Match.Stdin) {
			return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name},
				&StdinMismatchError{
					Scenario:  scn.Meta.Name,
					StepIndex: stepIndex,
					Expected:  expectedStep.Match.Stdin,
					Received:  actualStdin,
				}
		}
	}

	// Auto-advance CurrentStep if budget is now exhausted
	bounds := expectedStep.EffectiveCalls()
	if state.StepBudgetRemaining(stepIndex, bounds.Max) <= 0 {
		state.CurrentStep = stepIndex + 1
	}

	// Execute response with template rendering
	exitCode := ReplayResponseWithTemplate(expectedStep, scn, absPath, stdout, stderr)

	// Trace output if enabled
	if IsTraceEnabled(os.Getenv(TraceEnvVar)) {
		WriteTraceOutput(stderr, stepIndex, argv, exitCode)
	}

	// Save state (step count already incremented above, CurrentStep already advanced if needed)
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
	Scenario      string
	StepIndex     int
	Expected      []string
	Received      []string
	SoftAdvanced  bool     // true if we tried soft-advancing past a satisfied step
	NextStepIndex int      // index of the next step tried (when SoftAdvanced)
	NextExpected  []string // argv of the next step tried (when SoftAdvanced)
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("argv mismatch at step %d", e.StepIndex)
}

// StdinMismatchError represents a stdin content mismatch during replay.
type StdinMismatchError struct {
	Scenario  string
	StepIndex int
	Expected  string
	Received  string
}

func (e *StdinMismatchError) Error() string {
	return fmt.Sprintf("stdin mismatch at step %d", e.StepIndex)
}

// maxStdinBytes is the maximum number of bytes to read from stdin (1 MB).
const maxStdinBytes = 1 << 20

// readStdin reads stdin content up to maxStdinBytes.
func readStdin() (string, error) {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxStdinBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return string(data), nil
}

// normalizeStdin normalizes stdin content for comparison:
// converts \r\n to \n and trims trailing newlines.
func normalizeStdin(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}
