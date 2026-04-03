package replay

// Option configures the replay engine.
type Option func(*engineConfig)

type engineConfig struct {
	// vars are template variables merged into response rendering.
	// These override the scenario's meta.vars.
	vars map[string]string

	// denyEnvPatterns are glob patterns for env vars that must not
	// override template variables (e.g., "AWS_*", "SECRET_*").
	denyEnvPatterns []string

	// envLookup provides environment variable values for template rendering.
	// Defaults to nil (no env override). Set this to decouple from os.Getenv.
	envLookup func(string) string

	// fileReader provides file content for stdout_file/stderr_file responses.
	// Receives a path relative to the scenario directory.
	// If nil, file-based responses return an error.
	fileReader func(path string) (string, error)

	// matchFunc overrides the default argv matching function.
	// If nil, uses pkg/matcher.ArgvMatch.
	matchFunc func(expected, received []string) bool

	// initialState seeds the engine from a previously persisted snapshot.
	initialState *StateSnapshot
}

// WithVars sets additional template variables that override scenario meta.vars.
func WithVars(vars map[string]string) Option {
	return func(c *engineConfig) {
		c.vars = vars
	}
}

// WithDenyEnvPatterns sets glob patterns for environment variables that must
// not override template variables during rendering.
func WithDenyEnvPatterns(patterns []string) Option {
	return func(c *engineConfig) {
		c.denyEnvPatterns = patterns
	}
}

// WithEnvLookup sets the function used to resolve environment variables
// for template rendering. Pass nil to disable env var override entirely.
func WithEnvLookup(fn func(string) string) Option {
	return func(c *engineConfig) {
		c.envLookup = fn
	}
}

// WithFileReader sets the function used to read file content for
// stdout_file/stderr_file responses. The path argument is relative
// to the scenario directory.
func WithFileReader(fn func(path string) (string, error)) Option {
	return func(c *engineConfig) {
		c.fileReader = fn
	}
}

// WithMatchFunc overrides the default argv matching function.
// This is the extensibility point for custom matching strategies.
func WithMatchFunc(fn func(expected, received []string) bool) Option {
	return func(c *engineConfig) {
		c.matchFunc = fn
	}
}
