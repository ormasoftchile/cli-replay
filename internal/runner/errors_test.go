package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMismatchError_Error(t *testing.T) {
	err := &MismatchError{
		Scenario:  "test-scenario",
		StepIndex: 2,
		Expected:  []string{"kubectl", "get", "pods"},
		Received:  []string{"kubectl", "get", "services"},
	}

	errMsg := err.Error()
	assert.Contains(t, errMsg, "mismatch")
	assert.Contains(t, errMsg, "2")
}

func TestFormatMismatchError_LiteralDiff(t *testing.T) {
	// Force color off for predictable output
	t.Setenv("NO_COLOR", "1")

	err := &MismatchError{
		Scenario:  "my-scenario",
		StepIndex: 1, // 0-indexed; display should show "step 2"
		Expected:  []string{"kubectl", "get", "pods"},
		Received:  []string{"kubectl", "get", "services"},
	}

	formatted := FormatMismatchError(err)

	assert.Contains(t, formatted, `step 2 of "my-scenario"`)
	assert.Contains(t, formatted, "Expected:")
	assert.Contains(t, formatted, "Received:")
	assert.Contains(t, formatted, "position 2")
	assert.Contains(t, formatted, "pods")
	assert.Contains(t, formatted, "services")
}

func TestFormatMismatchError_RegexPatternDisplay(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	err := &MismatchError{
		Scenario:  "deployment-test",
		StepIndex: 1,
		Expected:  []string{"kubectl", "get", "pods", "-n", `{{ .regex "^prod-.*" }}`},
		Received:  []string{"kubectl", "get", "pods", "-n", "staging-app"},
	}

	formatted := FormatMismatchError(err)

	assert.Contains(t, formatted, "position 4")
	assert.Contains(t, formatted, "expected pattern")
	assert.Contains(t, formatted, "^prod-.*")
	assert.Contains(t, formatted, "staging-app")
}

func TestFormatMismatchError_WildcardSkip(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	// Wildcard should match anything, so diff should be at position after it
	err := &MismatchError{
		Scenario:  "test",
		StepIndex: 0,
		Expected:  []string{"cmd", "{{ .any }}", "expected"},
		Received:  []string{"cmd", "anything", "different"},
	}

	formatted := FormatMismatchError(err)

	// Wildcard at position 1 should be skipped; diff at position 2
	assert.Contains(t, formatted, "position 2")
	assert.NotContains(t, formatted, "position 1")
}

func TestFormatMismatchError_LengthMismatch_MissingArgs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	err := &MismatchError{
		Scenario:  "deployment-test",
		StepIndex: 0,
		Expected:  []string{"kubectl", "get", "pods", "-n", "{{ .any }}"},
		Received:  []string{"kubectl", "get", "pods"},
	}

	formatted := FormatMismatchError(err)

	assert.Contains(t, formatted, "(5 args)")
	assert.Contains(t, formatted, "(3 args)")
	assert.Contains(t, formatted, "Missing arguments starting at position 3")
	assert.Contains(t, formatted, `"-n"`)
}

func TestFormatMismatchError_LengthMismatch_ExtraArgs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	err := &MismatchError{
		Scenario:  "test",
		StepIndex: 0,
		Expected:  []string{"kubectl", "get"},
		Received:  []string{"kubectl", "get", "pods", "-n", "default"},
	}

	formatted := FormatMismatchError(err)

	assert.Contains(t, formatted, "Extra arguments starting at position 2")
	assert.Contains(t, formatted, `"pods"`)
}

func TestFormatMismatchError_LongArgvTruncation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	expected := make([]string, 15)
	received := make([]string, 15)
	for i := 0; i < 15; i++ {
		expected[i] = "arg"
		received[i] = "arg"
	}
	expected[14] = "expected-last"
	received[14] = "received-last"

	err := &MismatchError{
		Scenario:  "test",
		StepIndex: 0,
		Expected:  expected,
		Received:  received,
	}

	formatted := FormatMismatchError(err)

	// Should truncate at 12 elements and show "+3 more"
	assert.Contains(t, formatted, "+3 more")
}

func TestFormatMismatchError_IdenticalArrays(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	err := &MismatchError{
		Scenario:  "test",
		StepIndex: 0,
		Expected:  []string{"a", "b", "c"},
		Received:  []string{"a", "b", "c"},
	}

	formatted := FormatMismatchError(err)

	// No "First difference" section since they match
	assert.NotContains(t, formatted, "First difference")
	assert.NotContains(t, formatted, "Missing")
	assert.NotContains(t, formatted, "Extra")
}

func TestFindFirstDiff_TemplateAware(t *testing.T) {
	// findFirstDiff should skip wildcard and regex matches
	expected := []string{"cmd", "{{ .any }}", `{{ .regex "^prod" }}`, "literal"}
	received := []string{"cmd", "anything", "production", "different"}

	pos := findFirstDiff(expected, received)
	assert.Equal(t, 3, pos, "should skip wildcard at 1 and regex at 2, find diff at 3")
}

func TestFindFirstDiff_AllMatch(t *testing.T) {
	expected := []string{"cmd", "{{ .any }}", "literal"}
	received := []string{"cmd", "whatever", "literal"}

	pos := findFirstDiff(expected, received)
	assert.Equal(t, -1, pos)
}

func TestResolveColor_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLI_REPLAY_COLOR", "")
	assert.Equal(t, colorOff, resolveColor())
}

func TestResolveColor_CLIReplayColorOverride(t *testing.T) {
	t.Setenv("CLI_REPLAY_COLOR", "on")
	assert.Equal(t, colorOn, resolveColor())

	t.Setenv("CLI_REPLAY_COLOR", "off")
	assert.Equal(t, colorOff, resolveColor())
}

func TestFormatArgv_Short(t *testing.T) {
	result := formatArgv([]string{"a", "b", "c"})
	assert.Equal(t, "[a b c]", result)
}

func TestFormatArgv_Long(t *testing.T) {
	args := make([]string, 15)
	for i := range args {
		args[i] = "x"
	}
	result := formatArgv(args)
	assert.Contains(t, result, "+3 more")
}
