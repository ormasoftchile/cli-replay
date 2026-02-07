package runner

import (
	"fmt"
	"os"
	"strings"

	"github.com/cli-replay/cli-replay/internal/matcher"
	"golang.org/x/term"
)

// colorMode controls ANSI color output in error messages.
type colorMode int

const (
	colorAuto colorMode = iota
	colorOn
	colorOff
)

// resolveColor determines whether to emit ANSI color codes.
// Priority: CLI_REPLAY_COLOR env > NO_COLOR env > auto-detect stderr TTY.
func resolveColor() colorMode {
	if v := os.Getenv("CLI_REPLAY_COLOR"); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return colorOn
		case "0", "false", "no", "off":
			return colorOff
		}
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return colorOff
	}
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return colorOn
	}
	return colorOff
}

// ANSI escape helpers — return empty strings when color is off.
func red(s string, c colorMode) string {
	if c == colorOn {
		return "\033[31m" + s + "\033[0m"
	}
	return s
}

func green(s string, c colorMode) string {
	if c == colorOn {
		return "\033[32m" + s + "\033[0m"
	}
	return s
}

func bold(s string, c colorMode) string {
	if c == colorOn {
		return "\033[1m" + s + "\033[0m"
	}
	return s
}

// maxTruncatedArgv is the maximum number of argv elements shown before truncation.
const maxTruncatedArgv = 12

// formatArgv formats an argv slice for display, truncating if longer than maxTruncatedArgv.
func formatArgv(argv []string) string {
	if len(argv) <= maxTruncatedArgv {
		return fmt.Sprintf("%v", argv)
	}
	shown := make([]string, maxTruncatedArgv)
	copy(shown, argv[:maxTruncatedArgv])
	return fmt.Sprintf("%v  ...+%d more", shown, len(argv)-maxTruncatedArgv)
}

// FormatMismatchError formats a MismatchError for user-friendly output.
// Uses ElementMatchDetail for per-element diff, shows template patterns,
// and handles length mismatches with detailed position info.
func FormatMismatchError(err *MismatchError) string {
	color := resolveColor()
	var sb strings.Builder

	// Header: 1-based step number
	sb.WriteString(bold(fmt.Sprintf("Mismatch at step %d of %q:\n",
		err.StepIndex+1, err.Scenario), color))
	sb.WriteString("\n")

	// Full expected/received argv
	sb.WriteString(fmt.Sprintf("  Expected: %s", formatArgv(err.Expected)))
	if len(err.Expected) != len(err.Received) {
		sb.WriteString(fmt.Sprintf("  (%d args)", len(err.Expected)))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("  Received: %s", formatArgv(err.Received)))
	if len(err.Expected) != len(err.Received) {
		sb.WriteString(fmt.Sprintf("  (%d args)", len(err.Received)))
	}
	sb.WriteString("\n")

	// Find first divergence using element-level matching
	diffPos := findFirstDiff(err.Expected, err.Received)
	if diffPos >= 0 {
		sb.WriteString("\n")
		formatDiffDetail(&sb, err, diffPos, color)
	}

	// Soft-advance context
	if err.SoftAdvanced {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  Note: step %d min count was satisfied, tried advancing to step %d:\n",
			err.StepIndex+1, err.NextStepIndex+1))
		sb.WriteString(fmt.Sprintf("    Next expected: %s\n", formatArgv(err.NextExpected)))
		sb.WriteString("    Neither step matched the received command.\n")
	}

	return sb.String()
}

// formatDiffDetail appends the per-element diff detail to the builder.
func formatDiffDetail(sb *strings.Builder, err *MismatchError, diffPos int, color colorMode) {
	// Case 1: Both positions within bounds — element mismatch
	if diffPos < len(err.Expected) && diffPos < len(err.Received) {
		detail := matcher.ElementMatchDetail(err.Expected[diffPos], err.Received[diffPos])
		fmt.Fprintf(sb, "  First difference at position %d:\n", diffPos)

		switch detail.Kind {
		case "regex":
			fmt.Fprintf(sb, "    expected pattern: %s\n", red(detail.Pattern, color))
			fmt.Fprintf(sb, "    received value:   %s\n", red(err.Received[diffPos], color))
		case "wildcard":
			fmt.Fprintf(sb, "    expected: %s\n", err.Expected[diffPos])
			fmt.Fprintf(sb, "    received: %s\n", err.Received[diffPos])
		default: // literal
			fmt.Fprintf(sb, "    expected: %s\n", green(err.Expected[diffPos], color))
			fmt.Fprintf(sb, "    received: %s\n", red(err.Received[diffPos], color))
		}
		return
	}

	// Case 2: Received is longer — extra args
	if diffPos >= len(err.Expected) {
		fmt.Fprintf(sb, "  Extra arguments starting at position %d:\n", diffPos)
		limit := len(err.Received)
		if limit-diffPos > 5 {
			limit = diffPos + 5
		}
		for i := diffPos; i < limit; i++ {
			fmt.Fprintf(sb, "    [%d]: %s\n", i, red(fmt.Sprintf("%q", err.Received[i]), color))
		}
		if len(err.Received)-diffPos > 5 {
			fmt.Fprintf(sb, "    ...+%d more\n", len(err.Received)-diffPos-5)
		}
		return
	}

	// Case 3: Expected is longer — missing args
	fmt.Fprintf(sb, "  Missing arguments starting at position %d:\n", diffPos)
	limit := len(err.Expected)
	if limit-diffPos > 5 {
		limit = diffPos + 5
	}
	for i := diffPos; i < limit; i++ {
		fmt.Fprintf(sb, "    [%d]: %s\n", i, green(fmt.Sprintf("%q", err.Expected[i]), color))
	}
	if len(err.Expected)-diffPos > 5 {
		fmt.Fprintf(sb, "    ...+%d more\n", len(err.Expected)-diffPos-5)
	}
}

// findFirstDiff returns the index of the first element that does not match,
// using matcher.ElementMatchDetail for template-aware comparison.
// Returns -1 if all elements match and lengths are equal.
func findFirstDiff(expected, received []string) int {
	maxLen := len(expected)
	if len(received) > maxLen {
		maxLen = len(received)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(expected) || i >= len(received) {
			return i
		}
		detail := matcher.ElementMatchDetail(expected[i], received[i])
		if !detail.Matched {
			return i
		}
	}

	return -1
}

// maxStdinPreview is the maximum number of characters shown in stdin mismatch errors.
const maxStdinPreview = 200

// FormatStdinMismatchError formats a StdinMismatchError for user-friendly output.
func FormatStdinMismatchError(err *StdinMismatchError) string {
	color := resolveColor()
	var sb strings.Builder

	sb.WriteString(bold(fmt.Sprintf("Mismatch at step %d of %q:\n",
		err.StepIndex+1, err.Scenario), color))
	sb.WriteString("\n")
	sb.WriteString("  argv matched, stdin mismatch:\n")

	sb.WriteString(fmt.Sprintf("    expected (first %d chars):\n", maxStdinPreview))
	sb.WriteString(indentPreview(err.Expected, maxStdinPreview))

	sb.WriteString(fmt.Sprintf("    received (first %d chars):\n", maxStdinPreview))
	sb.WriteString(indentPreview(err.Received, maxStdinPreview))

	return sb.String()
}

// indentPreview returns the first n characters of s, indented with 6 spaces per line.
func indentPreview(s string, n int) string {
	if len(s) > n {
		s = s[:n] + "..."
	}
	var sb strings.Builder
	for _, line := range strings.Split(s, "\n") {
		sb.WriteString("      " + line + "\n")
	}
	return sb.String()
}
