package verify

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"testing"
	"time"

	"github.com/ormasoftchile/cli-replay/pkg/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Public API surface tests for pkg/verify — consumer-perspective tests
// documenting the verification contract.
// ═══════════════════════════════════════════════════════════════════════════════

// --- Budget-aware verification -----------------------------------------------

func TestBuildResult_BudgetAware_Table(t *testing.T) {
	tests := []struct {
		name        string
		steps       []scenario.Step
		stepCounts  []int
		wantPassed  bool
		wantConsum  int
		description string
	}{
		{
			name: "all required steps met exactly",
			steps: []scenario.Step{
				{Match: scenario.Match{Argv: []string{"cmd1"}}, Respond: scenario.Response{Exit: 0}},
				{Match: scenario.Match{Argv: []string{"cmd2"}}, Respond: scenario.Response{Exit: 0}},
			},
			stepCounts:  []int{1, 1},
			wantPassed:  true,
			wantConsum:  2,
			description: "default min=1, called once each",
		},
		{
			name: "optional step uncalled is still passing",
			steps: []scenario.Step{
				{Match: scenario.Match{Argv: []string{"required"}}, Respond: scenario.Response{Exit: 0}},
				{Match: scenario.Match{Argv: []string{"optional"}}, Respond: scenario.Response{Exit: 0},
					Calls: &scenario.CallBounds{Min: 0, Max: 3}},
			},
			stepCounts:  []int{1, 0},
			wantPassed:  true,
			wantConsum:  1,
			description: "min=0 means uncalled is OK",
		},
		{
			name: "multi-call step meets minimum",
			steps: []scenario.Step{
				{Match: scenario.Match{Argv: []string{"poll"}}, Respond: scenario.Response{Exit: 0},
					Calls: &scenario.CallBounds{Min: 3, Max: 10}},
			},
			stepCounts:  []int{5},
			wantPassed:  true,
			wantConsum:  1,
			description: "called 5 times, min is 3",
		},
		{
			name: "multi-call step below minimum fails",
			steps: []scenario.Step{
				{Match: scenario.Match{Argv: []string{"poll"}}, Respond: scenario.Response{Exit: 0},
					Calls: &scenario.CallBounds{Min: 3, Max: 10}},
			},
			stepCounts:  []int{2},
			wantPassed:  false,
			wantConsum:  1,
			description: "called 2 times, min is 3",
		},
		{
			name: "mixed required and optional",
			steps: []scenario.Step{
				{Match: scenario.Match{Argv: []string{"setup"}}, Respond: scenario.Response{Exit: 0}},
				{Match: scenario.Match{Argv: []string{"health"}}, Respond: scenario.Response{Exit: 0},
					Calls: &scenario.CallBounds{Min: 0, Max: 5}},
				{Match: scenario.Match{Argv: []string{"deploy"}}, Respond: scenario.Response{Exit: 0}},
			},
			stepCounts:  []int{1, 3, 1},
			wantPassed:  true,
			wantConsum:  3,
			description: "all minimums met across mixed step types",
		},
		{
			name: "empty step counts array",
			steps: []scenario.Step{
				{Match: scenario.Match{Argv: []string{"cmd"}}, Respond: scenario.Response{Exit: 0}},
			},
			stepCounts:  []int{},
			wantPassed:  false,
			wantConsum:  0,
			description: "no steps consumed at all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildResult("test", "default", tt.steps, tt.stepCounts, nil)

			assert.Equal(t, tt.wantPassed, result.Passed, tt.description)
			assert.Equal(t, tt.wantConsum, result.ConsumedSteps, "consumed steps")
			assert.Equal(t, len(tt.steps), result.TotalSteps)
		})
	}
}

// --- Group verification with labels -----------------------------------------

func TestBuildResult_GroupLabelsAndGroupField(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"setup"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"check", "dns"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"check", "api"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"deploy"}}, Respond: scenario.Response{Exit: 0}},
	}
	groupRanges := []scenario.GroupRange{
		{Start: 1, End: 3, Name: "pre-flight", TopIndex: 1},
	}
	result := BuildResult("test", "session-1", steps, []int{1, 1, 1, 1}, groupRanges)

	require.Len(t, result.Steps, 4)

	// Non-group steps
	assert.Empty(t, result.Steps[0].Group)
	assert.Equal(t, "setup", result.Steps[0].Label)
	assert.Empty(t, result.Steps[3].Group)
	assert.Equal(t, "deploy", result.Steps[3].Label)

	// Group steps
	assert.Equal(t, "pre-flight", result.Steps[1].Group)
	assert.Contains(t, result.Steps[1].Label, "check dns")
	assert.Contains(t, result.Steps[1].Label, "group:pre-flight")

	assert.Equal(t, "pre-flight", result.Steps[2].Group)
	assert.Contains(t, result.Steps[2].Label, "check api")
}

// --- JSON format: full round-trip with budget fields ------------------------

func TestFormatJSON_BudgetFields_Preserved(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"poll"}}, Respond: scenario.Response{Exit: 0},
			Calls: &scenario.CallBounds{Min: 2, Max: 10}},
	}
	state := []int{5}
	result := BuildResult("budget-test", "s1", steps, state, nil)

	var buf bytes.Buffer
	require.NoError(t, FormatJSON(&buf, result))

	var parsed VerifyResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	assert.True(t, parsed.Passed)
	require.Len(t, parsed.Steps, 1)
	assert.Equal(t, 5, parsed.Steps[0].CallCount)
	assert.Equal(t, 2, parsed.Steps[0].Min)
	assert.Equal(t, 10, parsed.Steps[0].Max)
}

// --- JUnit format: failure message content ----------------------------------

func TestFormatJUnit_FailedStep_MessageContent(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"cmd"}}, Respond: scenario.Response{Exit: 0},
			Calls: &scenario.CallBounds{Min: 3, Max: 5}},
	}
	state := []int{1}
	result := BuildResult("junit-fail", "default", steps, state, nil)

	var buf bytes.Buffer
	ts := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	require.NoError(t, FormatJUnit(&buf, result, "test.yaml", ts))

	var suites JUnitTestSuites
	require.NoError(t, xml.Unmarshal(buf.Bytes(), &suites))

	assert.Equal(t, 1, suites.Failures)
	require.Len(t, suites.Suites, 1)
	require.Len(t, suites.Suites[0].Cases, 1)
	tc := suites.Suites[0].Cases[0]
	require.NotNil(t, tc.Failure)
	assert.Contains(t, tc.Failure.Message, "called 1 times")
	assert.Contains(t, tc.Failure.Message, "minimum 3")
	assert.Equal(t, "VerificationFailure", tc.Failure.Type)
}

// --- JUnit format: optional skipped steps -----------------------------------

func TestFormatJUnit_OptionalStep_Skipped(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"required"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"optional"}}, Respond: scenario.Response{Exit: 0},
			Calls: &scenario.CallBounds{Min: 0, Max: 3}},
	}
	state := []int{1, 0}
	result := BuildResult("skip-test", "default", steps, state, nil)

	var buf bytes.Buffer
	ts := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	require.NoError(t, FormatJUnit(&buf, result, "test.yaml", ts))

	var suites JUnitTestSuites
	require.NoError(t, xml.Unmarshal(buf.Bytes(), &suites))

	assert.Equal(t, 0, suites.Failures)
	assert.Equal(t, 1, suites.Suites[0].Skipped)

	// Required step: no failure, no skip
	assert.Nil(t, suites.Suites[0].Cases[0].Failure)
	assert.Nil(t, suites.Suites[0].Cases[0].Skipped)

	// Optional step: skipped
	assert.Nil(t, suites.Suites[0].Cases[1].Failure)
	require.NotNil(t, suites.Suites[0].Cases[1].Skipped)
	assert.Contains(t, suites.Suites[0].Cases[1].Skipped.Message, "optional")
}

// --- JUnit format: error state -----------------------------------------------

func TestFormatJUnit_ErrorState_StructuredOutput(t *testing.T) {
	result := BuildErrorResult("err-scenario", "session-1", "no state found")

	var buf bytes.Buffer
	ts := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	require.NoError(t, FormatJUnit(&buf, result, "test.yaml", ts))

	var suites JUnitTestSuites
	require.NoError(t, xml.Unmarshal(buf.Bytes(), &suites))

	assert.Equal(t, 1, suites.Errors)
	assert.Equal(t, 0, suites.Failures)
	require.Len(t, suites.Suites[0].Cases, 1)
	assert.Equal(t, "StateError", suites.Suites[0].Cases[0].Failure.Type)
}

// --- StepLabel edge cases ----------------------------------------------------

func TestStepLabel_SingleArg(t *testing.T) {
	step := scenario.Step{Match: scenario.Match{Argv: []string{"cmd"}}}
	assert.Equal(t, "cmd", StepLabel(step))
}

func TestStepLabel_ArgsWithSpecialChars(t *testing.T) {
	step := scenario.Step{Match: scenario.Match{Argv: []string{"kubectl", "--namespace=kube-system", "-o", "json"}}}
	assert.Equal(t, "kubectl --namespace=kube-system -o json", StepLabel(step))
}
