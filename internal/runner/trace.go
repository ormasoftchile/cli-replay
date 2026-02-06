package runner

import (
	"fmt"
	"io"
	"strings"
)

// TraceEnvVar is the environment variable name for enabling trace mode.
const TraceEnvVar = "CLI_REPLAY_TRACE"

// WriteTraceOutput writes trace information to the given writer.
func WriteTraceOutput(w io.Writer, stepIndex int, argv []string, exitCode int) {
	_, _ = fmt.Fprintf(w, "[cli-replay] step=%d argv=%v exit=%d\n", stepIndex, argv, exitCode)
}

// IsTraceEnabled returns true if trace mode should be enabled.
func IsTraceEnabled(envValue string) bool {
	switch strings.ToLower(envValue) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
