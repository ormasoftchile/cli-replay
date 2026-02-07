package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// T020: TTL cleanup before matching (intercept shim path)
	if scn.Meta.Session != nil && scn.Meta.Session.TTL != "" {
		if ttl, parseErr := time.ParseDuration(scn.Meta.Session.TTL); parseErr == nil && ttl > 0 {
			cliReplayDir := filepath.Join(filepath.Dir(absPath), ".cli-replay")
			if cleaned, _ := CleanExpiredSessions(cliReplayDir, ttl, stderr); cleaned > 0 {
				_, _ = fmt.Fprintf(stderr, "cli-replay: cleaned %d expired sessions\n", cleaned)
			}
		}
	}

	// Flatten steps (expands groups inline) for sequential replay logic
	flatSteps := scn.FlatSteps()

	// Calculate scenario hash
	scenarioHash := hashScenarioFile(absPath)

	// Load or initialize state
	stateFile := StateFilePath(absPath)
	state, err := ReadState(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Initialize new state
			state = NewState(absPath, scenarioHash, len(flatSteps))
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
	groupRanges := scn.GroupRanges()

	// Phase 1: Skip exhausted steps (respects groups — skips entire group if all maxes hit)
	for stepIndex < len(flatSteps) {
		grIdx := FindGroupContaining(groupRanges, stepIndex)
		if grIdx >= 0 {
			// Inside a group — check if entire group is exhausted
			gr := groupRanges[grIdx]
			if state.GroupAllMaxesHit(gr, flatSteps) {
				stepIndex = gr.End // skip entire group
				state.ExitGroup()
				continue
			}
			break // group has budget; enter group matching
		}
		bounds := flatSteps[stepIndex].EffectiveCalls()
		if state.StepBudgetRemaining(stepIndex, bounds.Max) > 0 {
			break
		}
		stepIndex++
	}

	// Update CurrentStep if we skipped exhausted steps
	if stepIndex > state.CurrentStep {
		state.CurrentStep = stepIndex
	}

	if stepIndex >= len(flatSteps) {
		_, _ = fmt.Fprintf(stderr, "cli-replay: scenario %q already complete (all %d steps consumed)\n",
			scn.Meta.Name, state.TotalSteps)
		return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name},
			fmt.Errorf("scenario already complete")
	}

	// Determine if we're inside a group
	grIdx := FindGroupContaining(groupRanges, stepIndex)

	var matchedStep *scenario.Step
	var matchedIndex int

	if grIdx >= 0 {
		// ─── Group path: unordered matching ───
		gr := groupRanges[grIdx]
		state.EnterGroup(grIdx)

		// Linear scan all steps in group with remaining budget
		matchedIndex = -1
		for i := gr.Start; i < gr.End; i++ {
			bounds := flatSteps[i].EffectiveCalls()
			if state.StepBudgetRemaining(i, bounds.Max) <= 0 {
				continue // step exhausted
			}
			if matcher.ArgvMatch(flatSteps[i].Match.Argv, argv) {
				matchedIndex = i
				matchedStep = &flatSteps[i]
				break
			}
		}

		if matchedStep == nil {
			// No match in group — check if all mins are met
			if state.GroupAllMinsMet(gr, flatSteps) {
				// Soft-advance past group
				state.CurrentStep = gr.End
				state.ExitGroup()

				// Retry matching at the step after the group
				if gr.End < len(flatSteps) {
					retryStep := &flatSteps[gr.End]
					if matcher.ArgvMatch(retryStep.Match.Argv, argv) {
						matchedIndex = gr.End
						matchedStep = retryStep
					}
				}

				if matchedStep == nil {
					result := &ReplayResult{
						ExitCode:     1,
						Matched:      false,
						StepIndex:    gr.End,
						ScenarioName: scn.Meta.Name,
					}
					if gr.End < len(flatSteps) {
						return result, &MismatchError{
							Scenario:  scn.Meta.Name,
							StepIndex: gr.End,
							Expected:  flatSteps[gr.End].Match.Argv,
							Received:  argv,
						}
					}
					return result, fmt.Errorf("scenario already complete")
				}
			} else {
				// Mins not met — GroupMismatchError
				var candidates []int
				var candidateArgv [][]string
				for i := gr.Start; i < gr.End; i++ {
					bounds := flatSteps[i].EffectiveCalls()
					if state.StepBudgetRemaining(i, bounds.Max) > 0 {
						candidates = append(candidates, i)
						candidateArgv = append(candidateArgv, flatSteps[i].Match.Argv)
					}
				}
				return &ReplayResult{
					ExitCode:     1,
					Matched:      false,
					StepIndex:    stepIndex,
					ScenarioName: scn.Meta.Name,
				}, &GroupMismatchError{
					Scenario:      scn.Meta.Name,
					GroupName:     gr.Name,
					GroupIndex:    grIdx,
					Candidates:    candidates,
					CandidateArgv: candidateArgv,
					Received:      argv,
				}
			}
		}
	} else {
		// ─── Ordered path: existing logic ───
		expectedStep := &flatSteps[stepIndex]

		// Phase 2: Try matching current step
		matched := matcher.ArgvMatch(expectedStep.Match.Argv, argv)

		// Phase 3: Soft-advance if current step doesn't match but min is met
		softAdvanced := false
		origStepIndex := stepIndex
		if !matched {
			bounds := expectedStep.EffectiveCalls()
			if state.StepCounts != nil && stepIndex < len(state.StepCounts) &&
				state.StepCounts[stepIndex] >= bounds.Min && stepIndex+1 < len(flatSteps) {

				// Check if soft-advance would land in a group
				nextIdx := stepIndex + 1
				nextGrIdx := FindGroupContaining(groupRanges, nextIdx)
				if nextGrIdx >= 0 {
					// Soft-advance into a group — use group matching
					gr := groupRanges[nextGrIdx]
					state.CurrentStep = nextIdx
					state.EnterGroup(nextGrIdx)

					for i := gr.Start; i < gr.End; i++ {
						grBounds := flatSteps[i].EffectiveCalls()
						if state.StepBudgetRemaining(i, grBounds.Max) <= 0 {
							continue
						}
						if matcher.ArgvMatch(flatSteps[i].Match.Argv, argv) {
							matchedIndex = i
							matchedStep = &flatSteps[i]
							break
						}
					}

					if matchedStep == nil {
						// No match in the group either
						result := &ReplayResult{
							ExitCode:     1,
							Matched:      false,
							StepIndex:    origStepIndex,
							ScenarioName: scn.Meta.Name,
						}
						return result, &MismatchError{
							Scenario:     scn.Meta.Name,
							StepIndex:    origStepIndex,
							Expected:     flatSteps[origStepIndex].Match.Argv,
							Received:     argv,
							SoftAdvanced: true,
							NextStepIndex: nextIdx,
							NextExpected:  flatSteps[nextIdx].Match.Argv,
						}
					}
				} else {
					// Normal ordered soft-advance
					softAdvanced = true
					stepIndex++
					state.CurrentStep = stepIndex
					expectedStep = &flatSteps[stepIndex]
					matched = matcher.ArgvMatch(expectedStep.Match.Argv, argv)
				}
			}
		}

		if matchedStep == nil {
			// Handle result from ordered path (non-group)
			if matched {
				matchedIndex = stepIndex
				matchedStep = expectedStep
			} else {
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
					mErr.Expected = flatSteps[origStepIndex].Match.Argv
				}
				return result, mErr
			}
		}
	}

	// Increment call count for the matched step
	state.IncrementStep(matchedIndex)

	// stdin matching: if the step defines match.stdin, read actual stdin and compare
	if matchedStep.Match.Stdin != "" {
		actualStdin, readErr := readStdin()
		if readErr != nil {
			_, _ = fmt.Fprintf(stderr, "cli-replay: failed to read stdin: %v\n", readErr)
			return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name}, readErr
		}
		if normalizeStdin(actualStdin) != normalizeStdin(matchedStep.Match.Stdin) {
			return &ReplayResult{ExitCode: 1, ScenarioName: scn.Meta.Name},
				&StdinMismatchError{
					Scenario:  scn.Meta.Name,
					StepIndex: matchedIndex,
					Expected:  matchedStep.Match.Stdin,
					Received:  actualStdin,
				}
		}
	}

	// Auto-advance CurrentStep
	if grIdx >= 0 {
		gr := groupRanges[grIdx]
		// Inside a group — check if group is now fully exhausted
		if state.GroupAllMaxesHit(gr, flatSteps) {
			state.CurrentStep = gr.End
			state.ExitGroup()
		}
	} else {
		// Ordered path — advance if budget exhausted
		bounds := matchedStep.EffectiveCalls()
		if state.StepBudgetRemaining(matchedIndex, bounds.Max) <= 0 {
			state.CurrentStep = matchedIndex + 1
		}
	}

	// Execute response with template rendering (pass current captures for template resolution)
	exitCode := ReplayResponseWithTemplate(matchedStep, scn, absPath, state.Captures, stdout, stderr)

	// Merge step captures into state (T017: after response is served)
	// This naturally handles T018 (group captures — only captures from executed steps are merged)
	// and T019 (optional steps — captures merge only on invocation)
	if len(matchedStep.Respond.Capture) > 0 {
		if state.Captures == nil {
			state.Captures = make(map[string]string)
		}
		for k, v := range matchedStep.Respond.Capture {
			state.Captures[k] = v
		}
	}

	// Trace output if enabled
	if IsTraceEnabled(os.Getenv(TraceEnvVar)) {
		WriteTraceOutput(stderr, matchedIndex, argv, exitCode)
	}

	// Save state (step count already incremented above, CurrentStep already advanced if needed)
	if err := WriteState(stateFile, state); err != nil {
		_, _ = fmt.Fprintf(stderr, "cli-replay: warning: failed to save state: %v\n", err)
	}

	return &ReplayResult{
		ExitCode:     exitCode,
		Matched:      true,
		StepIndex:    matchedIndex,
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
