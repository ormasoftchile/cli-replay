package verify

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatJSON_AllPassed(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{TotalSteps: 2, StepCounts: []int{1, 1}}
	result := BuildResult("deploy-app", "default", steps, state, nil)

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	// Verify valid JSON
	var parsed VerifyResult
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.True(t, parsed.Passed)
	assert.Equal(t, "deploy-app", parsed.Scenario)
	assert.Equal(t, "default", parsed.Session)
	assert.Equal(t, 2, parsed.TotalSteps)
	assert.Equal(t, 2, parsed.ConsumedSteps)
	assert.Empty(t, parsed.Error)
	assert.Len(t, parsed.Steps, 2)
}

func TestFormatJSON_IncompleteSteps(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"kubectl", "apply", "-f", "app.yaml"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{TotalSteps: 2, StepCounts: []int{1, 0}}
	result := BuildResult("deploy-app", "default", steps, state, nil)

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	var parsed VerifyResult
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.False(t, parsed.Passed)
	assert.Equal(t, 1, parsed.ConsumedSteps)
	assert.True(t, parsed.Steps[0].Passed)
	assert.False(t, parsed.Steps[1].Passed)
}

func TestFormatJSON_NoStateError(t *testing.T) {
	result := BuildErrorResult("deploy-app", "default", "no state found")

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	var parsed VerifyResult
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.False(t, parsed.Passed)
	assert.Equal(t, 0, parsed.TotalSteps)
	assert.Equal(t, "no state found", parsed.Error)
	assert.Empty(t, parsed.Steps)
}

func TestFormatJSON_GroupPrefixedLabels(t *testing.T) {
	// Simulate a result with group-prefixed labels (set manually for now)
	result := &VerifyResult{
		Scenario:      "deploy-app",
		Session:       "default",
		Passed:        true,
		TotalSteps:    2,
		ConsumedSteps: 2,
		Steps: []StepResult{
			{Index: 0, Label: "[group:pre-flight] az account show", Group: "pre-flight", CallCount: 1, Min: 1, Max: 1, Passed: true},
			{Index: 1, Label: "[group:pre-flight] docker info", Group: "pre-flight", CallCount: 1, Min: 1, Max: 1, Passed: true},
		},
	}

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	// Round-trip through JSON
	var parsed VerifyResult
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "pre-flight", parsed.Steps[0].Group)
	assert.Equal(t, "[group:pre-flight] az account show", parsed.Steps[0].Label)
	assert.Equal(t, "pre-flight", parsed.Steps[1].Group)
	assert.Equal(t, "[group:pre-flight] docker info", parsed.Steps[1].Label)
}

func TestFormatJSON_CompactEncoding(t *testing.T) {
	result := &VerifyResult{
		Scenario:      "test",
		Session:       "default",
		Passed:        true,
		TotalSteps:    1,
		ConsumedSteps: 1,
		Steps: []StepResult{
			{Index: 0, Label: "git status", CallCount: 1, Min: 1, Max: 1, Passed: true},
		},
	}

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	// Compact JSON should be a single line (with trailing newline from Encode)
	output := buf.String()
	lines := bytes.Count([]byte(output), []byte("\n"))
	assert.Equal(t, 1, lines, "compact JSON should have exactly one trailing newline")
}

func TestFormatJSON_OmitsEmptyGroup(t *testing.T) {
	result := &VerifyResult{
		Scenario:      "test",
		Session:       "default",
		Passed:        true,
		TotalSteps:    1,
		ConsumedSteps: 1,
		Steps: []StepResult{
			{Index: 0, Label: "git status", CallCount: 1, Min: 1, Max: 1, Passed: true},
		},
	}

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	// The group field should be omitted in JSON when empty
	assert.NotContains(t, buf.String(), `"group"`)
}

func TestFormatJSON_OmitsEmptyError(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{TotalSteps: 1, StepCounts: []int{1}}
	result := BuildResult("test", "default", steps, state, nil)

	var buf bytes.Buffer
	err := FormatJSON(&buf, result)
	require.NoError(t, err)

	// The error field should be omitted when empty
	assert.NotContains(t, buf.String(), `"error"`)
}
