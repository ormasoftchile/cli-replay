package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cli-replay/cli-replay/internal/template"
	"github.com/cli-replay/cli-replay/pkg/replay"
	"github.com/cli-replay/cli-replay/pkg/scenario"
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
// Templates in stdout/stderr are rendered with vars from scenario meta + environment,
// and captures from prior steps via the "capture" template namespace.
// If deny_env_vars is configured, denied env vars are suppressed and traced.
func ReplayResponseWithTemplate(step *scenario.Step, scn *scenario.Scenario, scenarioPath string, captures map[string]string, stdout, stderr io.Writer) int {
	scenarioDir := filepath.Dir(scenarioPath)

	// Determine deny patterns from security config (T014, T015)
	var denyPatterns []string
	if scn.Meta.Security != nil && len(scn.Meta.Security.DenyEnvVars) > 0 {
		denyPatterns = scn.Meta.Security.DenyEnvVars
	}

	// Use filtered merge when deny patterns exist, else default behavior
	var vars map[string]string
	if len(denyPatterns) > 0 {
		var denied []string
		vars, denied = template.MergeVarsFiltered(scn.Meta.Vars, denyPatterns)
		// T010: Trace denied env vars
		if IsTraceEnabled(os.Getenv(TraceEnvVar)) {
			for _, name := range denied {
				WriteDeniedEnvTrace(stderr, name)
			}
		}
	} else {
		vars = template.MergeVars(scn.Meta.Vars)
	}

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
		rendered, err := template.RenderWithCaptures(stdoutContent, vars, captures)
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
		rendered, err := template.RenderWithCaptures(stderrContent, vars, captures)
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
// It loads the scenario, checks/creates state, delegates matching to
// pkg/replay.Engine, writes response output, and persists state.
//
//nolint:funlen // Orchestration function with many I/O steps
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

	// T020: TTL cleanup before matching (intercept shim path)
	if scn.Meta.Session != nil && scn.Meta.Session.TTL != "" {
		if ttl, parseErr := time.ParseDuration(scn.Meta.Session.TTL); parseErr == nil && ttl > 0 {
			cliReplayDir := filepath.Join(filepath.Dir(absPath), ".cli-replay")
			if cleaned, _ := CleanExpiredSessions(cliReplayDir, ttl, stderr); cleaned > 0 {
				_, _ = fmt.Fprintf(stderr, "cli-replay: cleaned %d expired sessions\n", cleaned)
			}
		}
	}

	flatSteps := scn.FlatSteps()
	scenarioHash := hashScenarioFile(absPath)
	scenarioDir := filepath.Dir(absPath)

	// Load or initialize persisted state
	stateFile := StateFilePath(absPath)
	state, err := ReadState(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			state = NewState(absPath, scenarioHash, len(flatSteps))
		} else {
			return &ReplayResult{ExitCode: 1}, fmt.Errorf("failed to read state: %w", err)
		}
	}

	// Check if scenario completed (early exit before creating engine)
	if state.IsComplete() {
		_, _ = fmt.Fprintf(stderr, "cli-replay: scenario %q already complete (all %d steps consumed)\n",
			scn.Meta.Name, state.TotalSteps)
		return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name},
			fmt.Errorf("scenario already complete")
	}

	// Build engine options
	opts := buildEngineOpts(scn, absPath, scenarioDir, state, stderr)

	engine := replay.New(scn, opts...)

	// Determine command name and args
	var name string
	var args []string
	if len(argv) > 0 {
		name = argv[0]
		args = argv[1:]
	}

	// Execute match — handle stdin if the matched step requires it
	result, matchErr := engine.Match(context.Background(), name, args)

	// If argv matched but we need to also validate stdin, re-check.
	// The engine already did argv matching; we handle stdin at this layer
	// because stdin reading requires os.Stdin (file I/O).
	if matchErr == nil && result.Matched {
		matchedIdx := result.StepIndex
		if matchedIdx < len(flatSteps) && flatSteps[matchedIdx].Match.Stdin != "" {
			actualStdin, readErr := readStdin()
			if readErr != nil {
				_, _ = fmt.Fprintf(stderr, "cli-replay: failed to read stdin: %v\n", readErr)
				return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name}, readErr
			}
			if normalizeStdin(actualStdin) != normalizeStdin(flatSteps[matchedIdx].Match.Stdin) {
				return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name},
					&StdinMismatchError{
						Scenario:  scn.Meta.Name,
						StepIndex: matchedIdx,
						Expected:  flatSteps[matchedIdx].Match.Stdin,
						Received:  actualStdin,
					}
			}
		}
	}

	// Convert engine errors to runner error types (preserves backward compat)
	if matchErr != nil {
		return convertEngineError(matchErr, scn.Meta.Name, state, stateFile)
	}

	// Write response to stdout/stderr
	if result.Stdout != "" {
		_, _ = io.WriteString(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		_, _ = io.WriteString(stderr, result.Stderr)
	}

	// Sync engine state back to persisted state
	snap := engine.Snapshot()
	state.CurrentStep = snap.CurrentStep
	state.StepCounts = snap.StepCounts
	state.Captures = snap.Captures
	if snap.ActiveGroup != nil {
		state.ActiveGroup = snap.ActiveGroup
	} else {
		state.ActiveGroup = nil
	}
	state.LastUpdated = time.Now().UTC()

	// Trace output if enabled
	if IsTraceEnabled(os.Getenv(TraceEnvVar)) {
		WriteTraceOutput(stderr, result.StepIndex, argv, result.ExitCode)
	}

	// Save state
	if err := WriteState(stateFile, state); err != nil {
		_, _ = fmt.Fprintf(stderr, "cli-replay: warning: failed to save state: %v\n", err)
	}

	return &ReplayResult{
		ExitCode:     result.ExitCode,
		Matched:      result.Matched,
		StepIndex:    result.StepIndex,
		ScenarioName: scn.Meta.Name,
	}, nil
}

// buildEngineOpts constructs replay.Option slice from scenario config and persisted state.
func buildEngineOpts(scn *scenario.Scenario, absPath, scenarioDir string, state *State, stderr io.Writer) []replay.Option {
	var opts []replay.Option

	// Seed engine with persisted state
	opts = append(opts, replay.WithInitialState(replay.StateSnapshot{
		CurrentStep: state.CurrentStep,
		TotalSteps:  state.TotalSteps,
		StepCounts:  state.StepCounts,
		ActiveGroup: state.ActiveGroup,
		Captures:    state.Captures,
	}))

	// Environment variable lookup (uses os.Getenv)
	opts = append(opts, replay.WithEnvLookup(os.Getenv))

	// Deny env patterns from security config
	if scn.Meta.Security != nil && len(scn.Meta.Security.DenyEnvVars) > 0 {
		opts = append(opts, replay.WithDenyEnvPatterns(scn.Meta.Security.DenyEnvVars))
	}

	// File reader for stdout_file/stderr_file
	opts = append(opts, replay.WithFileReader(func(relPath string) (string, error) {
		return readFile(scenarioDir, relPath)
	}))

	return opts
}

// convertEngineError maps pkg/replay error types to internal/runner error types
// for backward compatibility with existing CLI error formatting.
func convertEngineError(err error, scenarioName string, state *State, stateFile string) (*ReplayResult, error) {
	switch e := err.(type) {
	case *replay.MismatchError:
		return &ReplayResult{
			ExitCode:     1,
			Matched:      false,
			StepIndex:    e.StepIndex,
			ScenarioName: scenarioName,
		}, &MismatchError{
			Scenario:      scenarioName,
			StepIndex:     e.StepIndex,
			Expected:      e.Expected,
			Received:      e.Received,
			SoftAdvanced:  e.SoftAdvanced,
			NextStepIndex: e.NextStepIndex,
			NextExpected:  e.NextExpected,
		}
	case *replay.GroupMismatchError:
		return &ReplayResult{
			ExitCode:     1,
			Matched:      false,
			ScenarioName: scenarioName,
		}, &GroupMismatchError{
			Scenario:      scenarioName,
			GroupName:     e.GroupName,
			GroupIndex:    e.GroupIndex,
			Candidates:    e.Candidates,
			CandidateArgv: e.CandidateArgv,
			Received:      e.Received,
		}
	case *replay.ScenarioCompleteError:
		return &ReplayResult{
			ExitCode:     1,
			ScenarioName: scenarioName,
		}, fmt.Errorf("scenario already complete")
	default:
		return &ReplayResult{ExitCode: 1, ScenarioName: scenarioName}, err
	}
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

// GroupMismatchError is returned when a command does not match any step
// within an unordered group and the group's minimum counts are not yet met.
type GroupMismatchError struct {
	Scenario      string
	GroupName     string
	GroupIndex    int        // index into GroupRanges()
	Candidates    []int      // flat indices of unconsumed group steps
	CandidateArgv [][]string // argv of each candidate step
	Received      []string   // the received argv that didn't match
}

func (e *GroupMismatchError) Error() string {
	return fmt.Sprintf("no match in group %q (index %d): received %v",
		e.GroupName, e.GroupIndex, e.Received)
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
