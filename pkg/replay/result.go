// Package replay provides an in-memory replay engine for matching CLI commands
// against scenario steps. It is the public API for programmatic integration
// (e.g., from gert) and has zero file I/O, zero CLI coupling, and is thread-safe.
package replay

// Result contains the outcome of a single match operation.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int

	// StepIndex is the flat index of the matched step in the scenario.
	StepIndex int
	// Matched is true if the command was matched to a step.
	Matched bool
	// Captures accumulated after this match (snapshot, not a reference).
	Captures map[string]string
}
