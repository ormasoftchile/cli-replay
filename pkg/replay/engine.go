package replay

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/cli-replay/cli-replay/pkg/matcher"
	"github.com/cli-replay/cli-replay/pkg/scenario"
)

// Engine is an in-memory, thread-safe replay engine. It holds a loaded
// scenario and tracks which steps have been consumed. Given a command
// (name + args), it returns the matched response or an error.
//
// Engine has zero file I/O and zero CLI coupling — all I/O is provided
// through Option callbacks or via the scenario data pre-loaded at creation.
type Engine struct {
	mu         sync.Mutex
	scn        *scenario.Scenario
	flatSteps  []scenario.Step
	groupRanges []scenario.GroupRange
	st         *state
	cfg        engineConfig
}

// New creates a replay engine from a loaded scenario.
// The scenario is NOT copied — callers must not mutate it after passing it in.
func New(scn *scenario.Scenario, opts ...Option) *Engine {
	cfg := engineConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.matchFunc == nil {
		cfg.matchFunc = matcher.ArgvMatch
	}

	flat := scn.FlatSteps()
	st := newState(len(flat))

	// Restore state from snapshot if provided
	if cfg.initialState != nil {
		snap := cfg.initialState
		st.currentStep = snap.CurrentStep
		st.totalSteps = snap.TotalSteps
		if len(snap.StepCounts) == len(st.stepCounts) {
			copy(st.stepCounts, snap.StepCounts)
		}
		if snap.ActiveGroup != nil {
			v := *snap.ActiveGroup
			st.activeGroup = &v
		}
		if snap.Captures != nil {
			for k, v := range snap.Captures {
				st.captures[k] = v
			}
		}
	}

	return &Engine{
		scn:         scn,
		flatSteps:   flat,
		groupRanges: scn.GroupRanges(),
		st:          st,
		cfg:         cfg,
	}
}

// Match finds the best matching step for the given command and returns
// the rendered response. The context is reserved for future cancellation
// and timeout support.
//
//nolint:funlen,cyclop // Faithful extraction of the matching algorithm from internal/runner
func (e *Engine) Match(ctx context.Context, name string, args []string) (*Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	argv := append([]string{name}, args...)

	if e.st.isComplete() {
		return &Result{ExitCode: 1}, &ScenarioCompleteError{TotalSteps: e.st.totalSteps}
	}

	// Phase 1: Skip exhausted steps (respects groups)
	stepIndex := e.st.currentStep
	for stepIndex < len(e.flatSteps) {
		grIdx := findGroupContaining(e.groupRanges, stepIndex)
		if grIdx >= 0 {
			gr := e.groupRanges[grIdx]
			if e.st.groupAllMaxesHit(gr, e.flatSteps) {
				stepIndex = gr.End
				e.st.exitGroup()
				continue
			}
			break
		}
		bounds := e.flatSteps[stepIndex].EffectiveCalls()
		if e.st.stepBudgetRemaining(stepIndex, bounds.Max) > 0 {
			break
		}
		stepIndex++
	}

	if stepIndex > e.st.currentStep {
		e.st.currentStep = stepIndex
	}

	if stepIndex >= len(e.flatSteps) {
		return &Result{ExitCode: 1}, &ScenarioCompleteError{TotalSteps: e.st.totalSteps}
	}

	// Determine group membership
	grIdx := findGroupContaining(e.groupRanges, stepIndex)

	var matchedStep *scenario.Step
	var matchedIndex int

	if grIdx >= 0 {
		// ─── Group path: unordered matching ───
		matchedStep, matchedIndex = e.matchInGroup(grIdx, stepIndex, argv)

		if matchedStep == nil {
			gr := e.groupRanges[grIdx]
			if e.st.groupAllMinsMet(gr, e.flatSteps) {
				// Soft-advance past group
				e.st.currentStep = gr.End
				e.st.exitGroup()

				if gr.End < len(e.flatSteps) {
					retryStep := &e.flatSteps[gr.End]
					if e.cfg.matchFunc(retryStep.Match.Argv, argv) {
						matchedIndex = gr.End
						matchedStep = retryStep
					}
				}

				if matchedStep == nil {
					if gr.End < len(e.flatSteps) {
						return &Result{ExitCode: 1, StepIndex: gr.End},
							&MismatchError{
								StepIndex: gr.End,
								Expected:  e.flatSteps[gr.End].Match.Argv,
								Received:  argv,
							}
					}
					return &Result{ExitCode: 1}, &ScenarioCompleteError{TotalSteps: e.st.totalSteps}
				}
			} else {
				return e.groupMismatchResult(grIdx, argv)
			}
		}
	} else {
		// ─── Ordered path ───
		var mErr error
		matchedStep, matchedIndex, mErr = e.matchOrdered(stepIndex, argv)
		if mErr != nil {
			return &Result{ExitCode: 1, StepIndex: stepIndex}, mErr
		}
	}

	// By this point we have a valid match.

	// Increment call count
	e.st.incrementStep(matchedIndex)

	// Auto-advance CurrentStep
	if grIdx >= 0 {
		gr := e.groupRanges[grIdx]
		if e.st.groupAllMaxesHit(gr, e.flatSteps) {
			e.st.currentStep = gr.End
			e.st.exitGroup()
		}
	} else {
		bounds := matchedStep.EffectiveCalls()
		if e.st.stepBudgetRemaining(matchedIndex, bounds.Max) <= 0 {
			e.st.currentStep = matchedIndex + 1
		}
	}

	// Render response
	stdout, stderr, exitCode, err := e.renderResponse(matchedStep)
	if err != nil {
		return &Result{ExitCode: 1, StepIndex: matchedIndex, Matched: true}, err
	}

	// Merge captures
	if len(matchedStep.Respond.Capture) > 0 {
		for k, v := range matchedStep.Respond.Capture {
			e.st.captures[k] = v
		}
	}

	return &Result{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		StepIndex: matchedIndex,
		Matched:   true,
		Captures:  e.st.snapshotCaptures(),
	}, nil
}

// MatchWithStdin is like Match but also validates stdin content against
// the step's match.stdin field.
func (e *Engine) MatchWithStdin(ctx context.Context, name string, args []string, stdin string) (*Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	argv := append([]string{name}, args...)

	if e.st.isComplete() {
		return &Result{ExitCode: 1}, &ScenarioCompleteError{TotalSteps: e.st.totalSteps}
	}

	// Phase 1: Skip exhausted steps
	stepIndex := e.st.currentStep
	for stepIndex < len(e.flatSteps) {
		grIdx := findGroupContaining(e.groupRanges, stepIndex)
		if grIdx >= 0 {
			gr := e.groupRanges[grIdx]
			if e.st.groupAllMaxesHit(gr, e.flatSteps) {
				stepIndex = gr.End
				e.st.exitGroup()
				continue
			}
			break
		}
		bounds := e.flatSteps[stepIndex].EffectiveCalls()
		if e.st.stepBudgetRemaining(stepIndex, bounds.Max) > 0 {
			break
		}
		stepIndex++
	}

	if stepIndex > e.st.currentStep {
		e.st.currentStep = stepIndex
	}

	if stepIndex >= len(e.flatSteps) {
		return &Result{ExitCode: 1}, &ScenarioCompleteError{TotalSteps: e.st.totalSteps}
	}

	grIdx := findGroupContaining(e.groupRanges, stepIndex)

	var matchedStep *scenario.Step
	var matchedIndex int

	if grIdx >= 0 {
		matchedStep, matchedIndex = e.matchInGroup(grIdx, stepIndex, argv)

		if matchedStep == nil {
			gr := e.groupRanges[grIdx]
			if e.st.groupAllMinsMet(gr, e.flatSteps) {
				e.st.currentStep = gr.End
				e.st.exitGroup()

				if gr.End < len(e.flatSteps) {
					retryStep := &e.flatSteps[gr.End]
					if e.cfg.matchFunc(retryStep.Match.Argv, argv) {
						matchedIndex = gr.End
						matchedStep = retryStep
					}
				}

				if matchedStep == nil {
					if gr.End < len(e.flatSteps) {
						return &Result{ExitCode: 1, StepIndex: gr.End},
							&MismatchError{
								StepIndex: gr.End,
								Expected:  e.flatSteps[gr.End].Match.Argv,
								Received:  argv,
							}
					}
					return &Result{ExitCode: 1}, &ScenarioCompleteError{TotalSteps: e.st.totalSteps}
				}
			} else {
				return e.groupMismatchResult(grIdx, argv)
			}
		}
	} else {
		var mErr error
		matchedStep, matchedIndex, mErr = e.matchOrdered(stepIndex, argv)
		if mErr != nil {
			return &Result{ExitCode: 1, StepIndex: stepIndex}, mErr
		}
	}

	// Stdin validation
	if matchedStep.Match.Stdin != "" {
		if normalizeStdin(stdin) != normalizeStdin(matchedStep.Match.Stdin) {
			return &Result{ExitCode: 1},
				&StdinMismatchError{
					StepIndex: matchedIndex,
					Expected:  matchedStep.Match.Stdin,
					Received:  stdin,
				}
		}
	}

	e.st.incrementStep(matchedIndex)

	if grIdx >= 0 {
		gr := e.groupRanges[grIdx]
		if e.st.groupAllMaxesHit(gr, e.flatSteps) {
			e.st.currentStep = gr.End
			e.st.exitGroup()
		}
	} else {
		bounds := matchedStep.EffectiveCalls()
		if e.st.stepBudgetRemaining(matchedIndex, bounds.Max) <= 0 {
			e.st.currentStep = matchedIndex + 1
		}
	}

	stdout, stderr, exitCode, err := e.renderResponse(matchedStep)
	if err != nil {
		return &Result{ExitCode: 1, StepIndex: matchedIndex, Matched: true}, err
	}

	if len(matchedStep.Respond.Capture) > 0 {
		for k, v := range matchedStep.Respond.Capture {
			e.st.captures[k] = v
		}
	}

	return &Result{
		Stdout:    stdout,
		Stderr:    stderr,
		ExitCode:  exitCode,
		StepIndex: matchedIndex,
		Matched:   true,
		Captures:  e.st.snapshotCaptures(),
	}, nil
}

// Remaining returns the number of unconsumed steps.
func (e *Engine) Remaining() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.st.totalSteps - e.st.currentStep
	if r < 0 {
		return 0
	}
	return r
}

// StepCounts returns a snapshot of the per-step invocation counts.
func (e *Engine) StepCounts() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]int, len(e.st.stepCounts))
	copy(out, e.st.stepCounts)
	return out
}

// Captures returns a snapshot of accumulated captures.
func (e *Engine) Captures() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st.snapshotCaptures()
}

// Reset resets the engine to its initial state, allowing the scenario
// to be replayed from the beginning.
func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.st = newState(len(e.flatSteps))
}

// ─── internal helpers ───

// matchOrdered implements the ordered-path matching logic.
// Returns (matchedStep, matchedIndex, nil) on success, or (nil, idx, error) on mismatch.
func (e *Engine) matchOrdered(stepIndex int, argv []string) (*scenario.Step, int, error) {
	expectedStep := &e.flatSteps[stepIndex]
	matched := e.cfg.matchFunc(expectedStep.Match.Argv, argv)

	softAdvanced := false
	origStepIndex := stepIndex

	if !matched {
		bounds := expectedStep.EffectiveCalls()
		if stepIndex < len(e.st.stepCounts) &&
			e.st.stepCounts[stepIndex] >= bounds.Min && stepIndex+1 < len(e.flatSteps) {

			nextIdx := stepIndex + 1
			nextGrIdx := findGroupContaining(e.groupRanges, nextIdx)
			if nextGrIdx >= 0 {
				// Soft-advance into a group
				gr := e.groupRanges[nextGrIdx]
				e.st.currentStep = nextIdx
				e.st.enterGroup(nextGrIdx)

				for i := gr.Start; i < gr.End; i++ {
					grBounds := e.flatSteps[i].EffectiveCalls()
					if e.st.stepBudgetRemaining(i, grBounds.Max) <= 0 {
						continue
					}
					if e.cfg.matchFunc(e.flatSteps[i].Match.Argv, argv) {
						return &e.flatSteps[i], i, nil
					}
				}
				return nil, origStepIndex, &MismatchError{
					StepIndex:     origStepIndex,
					Expected:      e.flatSteps[origStepIndex].Match.Argv,
					Received:      argv,
					SoftAdvanced:  true,
					NextStepIndex: nextIdx,
					NextExpected:  e.flatSteps[nextIdx].Match.Argv,
				}
			}

			softAdvanced = true
			stepIndex++
			e.st.currentStep = stepIndex
			expectedStep = &e.flatSteps[stepIndex]
			matched = e.cfg.matchFunc(expectedStep.Match.Argv, argv)
		}
	}

	if matched {
		return expectedStep, stepIndex, nil
	}

	mErr := &MismatchError{
		StepIndex: stepIndex,
		Expected:  expectedStep.Match.Argv,
		Received:  argv,
	}
	if softAdvanced {
		mErr.SoftAdvanced = true
		mErr.NextStepIndex = stepIndex
		mErr.NextExpected = expectedStep.Match.Argv
		mErr.StepIndex = origStepIndex
		mErr.Expected = e.flatSteps[origStepIndex].Match.Argv
	}
	return nil, stepIndex, mErr
}

// matchInGroup implements unordered matching within a group.
func (e *Engine) matchInGroup(grIdx int, _ int, argv []string) (*scenario.Step, int) {
	gr := e.groupRanges[grIdx]
	e.st.enterGroup(grIdx)

	for i := gr.Start; i < gr.End; i++ {
		bounds := e.flatSteps[i].EffectiveCalls()
		if e.st.stepBudgetRemaining(i, bounds.Max) <= 0 {
			continue
		}
		if e.cfg.matchFunc(e.flatSteps[i].Match.Argv, argv) {
			return &e.flatSteps[i], i
		}
	}
	return nil, -1
}

func (e *Engine) groupMismatchResult(grIdx int, argv []string) (*Result, error) {
	gr := e.groupRanges[grIdx]
	var candidates []int
	var candidateArgv [][]string
	for i := gr.Start; i < gr.End; i++ {
		bounds := e.flatSteps[i].EffectiveCalls()
		if e.st.stepBudgetRemaining(i, bounds.Max) > 0 {
			candidates = append(candidates, i)
			candidateArgv = append(candidateArgv, e.flatSteps[i].Match.Argv)
		}
	}
	return &Result{ExitCode: 1, StepIndex: gr.Start},
		&GroupMismatchError{
			GroupName:     gr.Name,
			GroupIndex:    grIdx,
			Candidates:    candidates,
			CandidateArgv: candidateArgv,
			Received:      argv,
		}
}

// renderResponse renders the step's stdout/stderr with template variables and captures.
func (e *Engine) renderResponse(step *scenario.Step) (stdout, stderr string, exitCode int, err error) {
	vars := e.mergeVars()

	// Resolve stdout content
	stdoutContent := step.Respond.Stdout
	if step.Respond.StdoutFile != "" {
		if e.cfg.fileReader == nil {
			return "", "", 1, fmt.Errorf("stdout_file %q specified but no file reader configured", step.Respond.StdoutFile)
		}
		content, readErr := e.cfg.fileReader(step.Respond.StdoutFile)
		if readErr != nil {
			return "", "", 1, fmt.Errorf("failed to read stdout_file: %w", readErr)
		}
		stdoutContent = content
	}

	// Resolve stderr content
	stderrContent := step.Respond.Stderr
	if step.Respond.StderrFile != "" {
		if e.cfg.fileReader == nil {
			return "", "", 1, fmt.Errorf("stderr_file %q specified but no file reader configured", step.Respond.StderrFile)
		}
		content, readErr := e.cfg.fileReader(step.Respond.StderrFile)
		if readErr != nil {
			return "", "", 1, fmt.Errorf("failed to read stderr_file: %w", readErr)
		}
		stderrContent = content
	}

	// Render templates
	if stdoutContent != "" {
		stdoutContent, err = renderWithCaptures(stdoutContent, vars, e.st.captures)
		if err != nil {
			return "", "", 1, fmt.Errorf("failed to render stdout template: %w", err)
		}
	}
	if stderrContent != "" {
		stderrContent, err = renderWithCaptures(stderrContent, vars, e.st.captures)
		if err != nil {
			return "", "", 1, fmt.Errorf("failed to render stderr template: %w", err)
		}
	}

	return stdoutContent, stderrContent, step.Respond.Exit, nil
}

// mergeVars builds the template variable map: scenario meta.vars → option vars → env lookup.
func (e *Engine) mergeVars() map[string]string {
	result := make(map[string]string)

	// Base: scenario meta.vars
	for k, v := range e.scn.Meta.Vars {
		result[k] = v
	}

	// Override: option vars
	for k, v := range e.cfg.vars {
		result[k] = v
	}

	// Override: env lookup (if configured)
	if e.cfg.envLookup != nil {
		for k := range result {
			if envVal := e.cfg.envLookup(k); envVal != "" {
				// Check deny patterns
				if len(e.cfg.denyEnvPatterns) > 0 && isDenied(k, e.cfg.denyEnvPatterns) {
					continue
				}
				result[k] = envVal
			}
		}
	}

	return result
}

// renderWithCaptures is a self-contained template renderer (no dependency
// on internal/template) that renders Go text/templates with vars and captures.
func renderWithCaptures(tmpl string, vars map[string]string, captures map[string]string) (string, error) {
	if tmpl == "" {
		return "", nil
	}

	t, err := template.New("response").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	data := make(map[string]interface{}, len(vars)+1)
	for k, v := range vars {
		data[k] = v
	}

	captureMap := make(map[string]string)
	for k, v := range captures {
		captureMap[k] = v
	}
	data["capture"] = captureMap

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.String(), nil
}

// isDenied checks if a variable name matches any of the deny patterns.
// Supports glob-style matching (*, ?).
func isDenied(name string, patterns []string) bool {
	for _, p := range patterns {
		if globMatch(p, name) {
			return true
		}
	}
	return false
}

// globMatch is a simple glob matcher supporting * and ? wildcards.
func globMatch(pattern, name string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Try matching rest of pattern at every position
			pattern = pattern[1:]
			if pattern == "" {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if globMatch(pattern, name[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(name) == 0 {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		default:
			if len(name) == 0 || pattern[0] != name[0] {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		}
	}
	return len(name) == 0
}

// findGroupContaining returns the index into groupRanges that contains the
// given flat step index, or -1 if the index is not inside any group.
func findGroupContaining(ranges []scenario.GroupRange, flatIdx int) int {
	for i, gr := range ranges {
		if flatIdx >= gr.Start && flatIdx < gr.End {
			return i
		}
	}
	return -1
}

// normalizeStdin normalizes stdin content for comparison.
func normalizeStdin(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}
