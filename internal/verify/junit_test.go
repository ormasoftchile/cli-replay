package verify

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testTimestamp = time.Date(2026, 2, 7, 10, 30, 0, 0, time.UTC)

func TestFormatJUnit_AllPassed(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"kubectl", "apply", "-f", "app.yaml"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{TotalSteps: 3, StepCounts: []int{1, 1, 1}}
	result := BuildResult("deploy-app", "default", steps, state, nil)

	var buf bytes.Buffer
	err := FormatJUnit(&buf, result, "scenario.yaml", testTimestamp)
	require.NoError(t, err)

	output := buf.String()

	// Verify valid XML
	var parsed JUnitTestSuites
	err = xml.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "cli-replay", parsed.Name)
	assert.Equal(t, 3, parsed.Tests)
	assert.Equal(t, 0, parsed.Failures)
	assert.Equal(t, 0, parsed.Errors)
	require.Len(t, parsed.Suites, 1)

	suite := parsed.Suites[0]
	assert.Equal(t, "deploy-app", suite.Name)
	assert.Equal(t, 3, suite.Tests)
	assert.Equal(t, 0, suite.Failures)
	assert.Equal(t, 0, suite.Skipped)
	assert.Equal(t, "2026-02-07T10:30:00Z", suite.Timestamp)
	require.Len(t, suite.Cases, 3)

	// All cases should have no failure/skipped elements
	for _, tc := range suite.Cases {
		assert.Equal(t, "scenario.yaml", tc.Classname)
		assert.Equal(t, "0.000", tc.Time)
		assert.Nil(t, tc.Failure)
		assert.Nil(t, tc.Skipped)
	}

	assert.Equal(t, "step[0]: git status", suite.Cases[0].Name)
	assert.Equal(t, "step[1]: kubectl get pods", suite.Cases[1].Name)
	assert.Equal(t, "step[2]: kubectl apply -f app.yaml", suite.Cases[2].Name)
}

func TestFormatJUnit_FailureElements(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"docker", "info"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{TotalSteps: 2, StepCounts: []int{1, 0}}
	result := BuildResult("deploy-app", "default", steps, state, nil)

	var buf bytes.Buffer
	err := FormatJUnit(&buf, result, "scenario.yaml", testTimestamp)
	require.NoError(t, err)

	var parsed JUnitTestSuites
	err = xml.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Failures)
	suite := parsed.Suites[0]
	assert.Equal(t, 1, suite.Failures)

	// First case passes
	assert.Nil(t, suite.Cases[0].Failure)

	// Second case fails
	require.NotNil(t, suite.Cases[1].Failure)
	assert.Equal(t, "VerificationFailure", suite.Cases[1].Failure.Type)
	assert.Equal(t, "called 0 times, minimum 1 required", suite.Cases[1].Failure.Message)
	assert.Equal(t, "called 0 times, minimum 1 required", suite.Cases[1].Failure.Content)
}

func TestFormatJUnit_SkippedForMinZero(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
		{
			Match:   scenario.Match{Argv: []string{"docker", "info"}},
			Respond: scenario.Response{Exit: 0},
			Calls:   &scenario.CallBounds{Min: 0, Max: 3},
		},
	}
	state := &runner.State{TotalSteps: 2, StepCounts: []int{1, 0}}
	result := BuildResult("test", "default", steps, state, nil)

	var buf bytes.Buffer
	err := FormatJUnit(&buf, result, "scenario.yaml", testTimestamp)
	require.NoError(t, err)

	var parsed JUnitTestSuites
	err = xml.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	suite := parsed.Suites[0]
	assert.Equal(t, 0, suite.Failures)
	assert.Equal(t, 1, suite.Skipped)

	// Optional step should have skipped element
	assert.Nil(t, suite.Cases[1].Failure)
	require.NotNil(t, suite.Cases[1].Skipped)
	assert.Equal(t, "optional step (min=0), not called", suite.Cases[1].Skipped.Message)
}

func TestFormatJUnit_ErrorState(t *testing.T) {
	result := BuildErrorResult("deploy-app", "default", "no state found")

	var buf bytes.Buffer
	err := FormatJUnit(&buf, result, "scenario.yaml", testTimestamp)
	require.NoError(t, err)

	var parsed JUnitTestSuites
	err = xml.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 0, parsed.Tests)
	assert.Equal(t, 1, parsed.Errors)
	assert.Equal(t, 0, parsed.Failures)

	suite := parsed.Suites[0]
	assert.Equal(t, 1, suite.Errors)
	require.Len(t, suite.Cases, 1)

	tc := suite.Cases[0]
	assert.Equal(t, "state", tc.Name)
	require.NotNil(t, tc.Failure) // error uses failure element with StateError type
	assert.Equal(t, "StateError", tc.Failure.Type)
	assert.Equal(t, "no state found", tc.Failure.Message)
}

func TestFormatJUnit_XMLValidity(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{TotalSteps: 1, StepCounts: []int{1}}
	result := BuildResult("test", "default", steps, state, nil)

	var buf bytes.Buffer
	err := FormatJUnit(&buf, result, "scenario.yaml", testTimestamp)
	require.NoError(t, err)

	output := buf.String()

	// Must start with XML header
	assert.True(t, strings.HasPrefix(output, `<?xml version="1.0" encoding="UTF-8"?>`))

	// Must parse as valid XML
	var parsed JUnitTestSuites
	err = xml.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
}

func TestFormatJUnit_AttributeMapping(t *testing.T) {
	steps := []scenario.Step{
		{
			Match:   scenario.Match{Argv: []string{"kubectl", "get", "pods"}},
			Respond: scenario.Response{Exit: 0},
			Calls:   &scenario.CallBounds{Min: 2, Max: 5},
		},
	}
	state := &runner.State{TotalSteps: 1, StepCounts: []int{3}}
	result := BuildResult("my-scenario", "default", steps, state, nil)

	var buf bytes.Buffer
	err := FormatJUnit(&buf, result, "path/to/scenario.yaml", testTimestamp)
	require.NoError(t, err)

	var parsed JUnitTestSuites
	err = xml.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	// Top-level attributes
	assert.Equal(t, "cli-replay", parsed.Name)
	assert.Equal(t, 1, parsed.Tests)

	// Suite attributes
	suite := parsed.Suites[0]
	assert.Equal(t, "my-scenario", suite.Name)
	assert.Equal(t, "0.000", suite.Time)

	// Test case attributes
	tc := suite.Cases[0]
	assert.Equal(t, "step[0]: kubectl get pods", tc.Name)
	assert.Equal(t, "path/to/scenario.yaml", tc.Classname)
	assert.Equal(t, "0.000", tc.Time)
}

func TestFormatJUnit_GroupTestCaseName(t *testing.T) {
	// Simulate grouped steps with manual StepResult construction
	result := &VerifyResult{
		Scenario:      "deploy-app",
		Session:       "default",
		Passed:        true,
		TotalSteps:    2,
		ConsumedSteps: 2,
		Steps: []StepResult{
			{Index: 0, Label: "az account show", Group: "pre-flight", CallCount: 1, Min: 1, Max: 1, Passed: true},
			{Index: 1, Label: "docker info", Group: "pre-flight", CallCount: 1, Min: 1, Max: 1, Passed: true},
		},
	}

	var buf bytes.Buffer
	err := FormatJUnit(&buf, result, "scenario.yaml", testTimestamp)
	require.NoError(t, err)

	var parsed JUnitTestSuites
	err = xml.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	suite := parsed.Suites[0]
	assert.Equal(t, "[group:pre-flight] step[0]: az account show", suite.Cases[0].Name)
	assert.Equal(t, "[group:pre-flight] step[1]: docker info", suite.Cases[1].Name)
}

func TestStepTestCaseName(t *testing.T) {
	tests := []struct {
		name string
		step StepResult
		want string
	}{
		{
			"no group",
			StepResult{Index: 0, Label: "git status"},
			"step[0]: git status",
		},
		{
			"with group",
			StepResult{Index: 1, Label: "az account show", Group: "pre-flight"},
			"[group:pre-flight] step[1]: az account show",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stepTestCaseName(tt.step))
		})
	}
}
