package verify

import (
	"testing"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/stretchr/testify/assert"
)

func TestBuildResult_AllPassed(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{
		TotalSteps: 2,
		StepCounts: []int{1, 1},
	}

	result := BuildResult("test-scenario", "default", steps, state, nil)

	assert.True(t, result.Passed)
	assert.Equal(t, "test-scenario", result.Scenario)
	assert.Equal(t, "default", result.Session)
	assert.Equal(t, 2, result.TotalSteps)
	assert.Equal(t, 2, result.ConsumedSteps)
	assert.Empty(t, result.Error)
	assert.Len(t, result.Steps, 2)

	assert.Equal(t, 0, result.Steps[0].Index)
	assert.Equal(t, "git status", result.Steps[0].Label)
	assert.Equal(t, 1, result.Steps[0].CallCount)
	assert.Equal(t, 1, result.Steps[0].Min)
	assert.Equal(t, 1, result.Steps[0].Max)
	assert.True(t, result.Steps[0].Passed)

	assert.Equal(t, 1, result.Steps[1].Index)
	assert.Equal(t, "kubectl get pods", result.Steps[1].Label)
	assert.True(t, result.Steps[1].Passed)
}

func TestBuildResult_Incomplete(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"kubectl", "apply", "-f", "app.yaml"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{
		TotalSteps: 2,
		StepCounts: []int{1, 0},
	}

	result := BuildResult("deploy-app", "default", steps, state, nil)

	assert.False(t, result.Passed)
	assert.Equal(t, 2, result.TotalSteps)
	assert.Equal(t, 1, result.ConsumedSteps)
	assert.Empty(t, result.Error)
	assert.True(t, result.Steps[0].Passed)
	assert.False(t, result.Steps[1].Passed)
	assert.Equal(t, 0, result.Steps[1].CallCount)
}

func TestBuildResult_NoState(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
	}

	result := BuildResult("test-scenario", "default", steps, nil, nil)

	assert.False(t, result.Passed)
	assert.Equal(t, 0, result.TotalSteps)
	assert.Equal(t, 0, result.ConsumedSteps)
	assert.Equal(t, "no state found", result.Error)
	assert.NotNil(t, result.Steps)
	assert.Empty(t, result.Steps)
}

func TestBuildResult_OptionalStepMinZero(t *testing.T) {
	steps := []scenario.Step{
		{
			Match:   scenario.Match{Argv: []string{"git", "status"}},
			Respond: scenario.Response{Exit: 0},
		},
		{
			Match:   scenario.Match{Argv: []string{"docker", "info"}},
			Respond: scenario.Response{Exit: 0},
			Calls:   &scenario.CallBounds{Min: 0, Max: 3},
		},
	}
	state := &runner.State{
		TotalSteps: 2,
		StepCounts: []int{1, 0},
	}

	result := BuildResult("optional-test", "default", steps, state, nil)

	assert.True(t, result.Passed, "should pass because min=0 for step 1")
	assert.Equal(t, 2, result.TotalSteps)
	assert.Equal(t, 1, result.ConsumedSteps)
	assert.Empty(t, result.Error)

	// Step 0: normal step, called once, passes
	assert.True(t, result.Steps[0].Passed)
	assert.Equal(t, 1, result.Steps[0].CallCount)
	assert.Equal(t, 1, result.Steps[0].Min)
	assert.Equal(t, 1, result.Steps[0].Max)

	// Step 1: optional step (min=0), not called, still passes
	assert.True(t, result.Steps[1].Passed)
	assert.Equal(t, 0, result.Steps[1].CallCount)
	assert.Equal(t, 0, result.Steps[1].Min)
	assert.Equal(t, 3, result.Steps[1].Max)
}

func TestBuildErrorResult(t *testing.T) {
	result := BuildErrorResult("my-scenario", "session-1", "no state found")

	assert.False(t, result.Passed)
	assert.Equal(t, "my-scenario", result.Scenario)
	assert.Equal(t, "session-1", result.Session)
	assert.Equal(t, 0, result.TotalSteps)
	assert.Equal(t, 0, result.ConsumedSteps)
	assert.Equal(t, "no state found", result.Error)
	assert.NotNil(t, result.Steps)
	assert.Empty(t, result.Steps)
}

func TestStepLabel(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"single arg", []string{"git"}, "git"},
		{"two args", []string{"git", "status"}, "git status"},
		{"multi args", []string{"kubectl", "apply", "-f", "app.yaml"}, "kubectl apply -f app.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := scenario.Step{Match: scenario.Match{Argv: tt.argv}}
			assert.Equal(t, tt.want, StepLabel(step))
		})
	}
}

func TestBuildResult_MultiCallStep(t *testing.T) {
	steps := []scenario.Step{
		{
			Match:   scenario.Match{Argv: []string{"kubectl", "get", "pods"}},
			Respond: scenario.Response{Exit: 0},
			Calls:   &scenario.CallBounds{Min: 2, Max: 5},
		},
	}
	state := &runner.State{
		TotalSteps: 1,
		StepCounts: []int{3},
	}

	result := BuildResult("multi-call", "default", steps, state, nil)

	assert.True(t, result.Passed)
	assert.Equal(t, 1, result.TotalSteps)
	assert.Equal(t, 1, result.ConsumedSteps)
	assert.Equal(t, 3, result.Steps[0].CallCount)
	assert.Equal(t, 2, result.Steps[0].Min)
	assert.Equal(t, 5, result.Steps[0].Max)
	assert.True(t, result.Steps[0].Passed)
}

func TestBuildResult_GroupFieldEmpty(t *testing.T) {
	// Group field should be empty string (omitted in JSON) for non-group steps
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"git", "status"}}, Respond: scenario.Response{Exit: 0}},
	}
	state := &runner.State{
		TotalSteps: 1,
		StepCounts: []int{1},
	}

	result := BuildResult("test", "default", steps, state, nil)

	assert.Empty(t, result.Steps[0].Group, "group should be empty for non-group steps")
}

func TestBuildResult_GroupFieldPopulated(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"setup"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"check", "a"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"check", "b"}}, Respond: scenario.Response{Exit: 0}},
		{Match: scenario.Match{Argv: []string{"deploy"}}, Respond: scenario.Response{Exit: 0}},
	}
	groupRanges := []scenario.GroupRange{
		{Start: 1, End: 3, Name: "pre-flight", TopIndex: 1},
	}
	state := &runner.State{
		TotalSteps: 4,
		StepCounts: []int{1, 1, 1, 1},
	}

	result := BuildResult("test", "default", steps, state, groupRanges)

	// Step 0: outside group
	assert.Empty(t, result.Steps[0].Group)
	assert.Equal(t, "setup", result.Steps[0].Label)

	// Steps 1-2: inside group
	assert.Equal(t, "pre-flight", result.Steps[1].Group)
	assert.Equal(t, "[group:pre-flight] check a", result.Steps[1].Label)

	assert.Equal(t, "pre-flight", result.Steps[2].Group)
	assert.Equal(t, "[group:pre-flight] check b", result.Steps[2].Label)

	// Step 3: outside group
	assert.Empty(t, result.Steps[3].Group)
	assert.Equal(t, "deploy", result.Steps[3].Label)
}
