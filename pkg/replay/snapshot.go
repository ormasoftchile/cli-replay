package replay

// StateSnapshot is a serializable representation of the engine's internal
// state. Use it to persist state between invocations (e.g., to file) or
// to initialise an engine from previously saved progress.
type StateSnapshot struct {
	CurrentStep int
	TotalSteps  int
	StepCounts  []int
	ActiveGroup *int
	Captures    map[string]string
}

// WithInitialState seeds the engine with a previously persisted state snapshot.
// This allows the caller to resume a replay session across process boundaries.
func WithInitialState(snap StateSnapshot) Option {
	return func(c *engineConfig) {
		c.initialState = &snap
	}
}

// Snapshot returns a copy of the engine's current state for external persistence.
func (e *Engine) Snapshot() StateSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()

	var ag *int
	if e.st.activeGroup != nil {
		v := *e.st.activeGroup
		ag = &v
	}

	counts := make([]int, len(e.st.stepCounts))
	copy(counts, e.st.stepCounts)

	return StateSnapshot{
		CurrentStep: e.st.currentStep,
		TotalSteps:  e.st.totalSteps,
		StepCounts:  counts,
		ActiveGroup: ag,
		Captures:    e.st.snapshotCaptures(),
	}
}
