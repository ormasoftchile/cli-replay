package runner

import (
	"fmt"
	"strings"
)

// FormatMismatchError formats a MismatchError for user-friendly output.
func FormatMismatchError(err *MismatchError) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("cli-replay: argv mismatch in scenario %q at step %d\n",
		err.Scenario, err.StepIndex))
	sb.WriteString(fmt.Sprintf("  expected: %v\n", err.Expected))
	sb.WriteString(fmt.Sprintf("  received: %v\n", err.Received))

	// Show first differing position
	diffPos := findFirstDiff(err.Expected, err.Received)
	if diffPos >= 0 {
		sb.WriteString(fmt.Sprintf("  first difference at position %d:\n", diffPos))
		if diffPos < len(err.Expected) {
			sb.WriteString(fmt.Sprintf("    expected[%d]: %q\n", diffPos, err.Expected[diffPos]))
		} else {
			sb.WriteString(fmt.Sprintf("    expected[%d]: (missing)\n", diffPos))
		}
		if diffPos < len(err.Received) {
			sb.WriteString(fmt.Sprintf("    received[%d]: %q\n", diffPos, err.Received[diffPos]))
		} else {
			sb.WriteString(fmt.Sprintf("    received[%d]: (missing)\n", diffPos))
		}
	}

	return sb.String()
}

// findFirstDiff returns the index of the first differing element, or -1 if equal.
func findFirstDiff(a, b []string) int {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(a) || i >= len(b) {
			return i
		}
		if a[i] != b[i] {
			return i
		}
	}

	return -1
}
