package replay

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cli-replay/cli-replay/pkg/scenario"
)

// helper to build a minimal scenario from flat steps
func buildScenario(name string, steps ...scenario.StepElement) *scenario.Scenario {
	return &scenario.Scenario{
		Meta:  scenario.Meta{Name: name},
		Steps: steps,
	}
}

func leafStep(argv []string, stdout string, exit int) scenario.StepElement {
	return scenario.StepElement{
		Step: &scenario.Step{
			Match:   scenario.Match{Argv: argv},
			Respond: scenario.Response{Exit: exit, Stdout: stdout},
		},
	}
}

func leafStepWithCalls(argv []string, stdout string, exit, min, max int) scenario.StepElement {
	return scenario.StepElement{
		Step: &scenario.Step{
			Match:   scenario.Match{Argv: argv},
			Respond: scenario.Response{Exit: exit, Stdout: stdout},
			Calls:   &scenario.CallBounds{Min: min, Max: max},
		},
	}
}

func leafStepWithCapture(argv []string, stdout string, exit int, captures map[string]string) scenario.StepElement {
	return scenario.StepElement{
		Step: &scenario.Step{
			Match:   scenario.Match{Argv: argv},
			Respond: scenario.Response{Exit: exit, Stdout: stdout, Capture: captures},
		},
	}
}

func groupStep(name string, children ...scenario.StepElement) scenario.StepElement {
	return scenario.StepElement{
		Group: &scenario.StepGroup{
			Mode:  "unordered",
			Name:  name,
			Steps: children,
		},
	}
}

func TestEngine_SingleStep(t *testing.T) {
	scn := buildScenario("test",
		leafStep([]string{"kubectl", "get", "pods"}, "pod-list\n", 0),
	)
	eng := New(scn)

	result, err := eng.Match(context.Background(), "kubectl", []string{"get", "pods"})
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "pod-list\n", result.Stdout)
	assert.Equal(t, 0, result.StepIndex)
	assert.Equal(t, 0, eng.Remaining())
}

func TestEngine_MultiStepOrdered(t *testing.T) {
	scn := buildScenario("multi",
		leafStep([]string{"cmd", "first"}, "first\n", 0),
		leafStep([]string{"cmd", "second"}, "second\n", 0),
		leafStep([]string{"cmd", "third"}, "third\n", 0),
	)
	eng := New(scn)
	ctx := context.Background()

	r1, err := eng.Match(ctx, "cmd", []string{"first"})
	require.NoError(t, err)
	assert.Equal(t, "first\n", r1.Stdout)
	assert.Equal(t, 2, eng.Remaining())

	r2, err := eng.Match(ctx, "cmd", []string{"second"})
	require.NoError(t, err)
	assert.Equal(t, "second\n", r2.Stdout)

	r3, err := eng.Match(ctx, "cmd", []string{"third"})
	require.NoError(t, err)
	assert.Equal(t, "third\n", r3.Stdout)
	assert.Equal(t, 0, eng.Remaining())
}

func TestEngine_Mismatch(t *testing.T) {
	scn := buildScenario("test",
		leafStep([]string{"cmd", "expected"}, "output", 0),
	)
	eng := New(scn)

	_, err := eng.Match(context.Background(), "cmd", []string{"wrong"})
	require.Error(t, err)

	var mErr *MismatchError
	require.ErrorAs(t, err, &mErr)
	assert.Equal(t, []string{"cmd", "expected"}, mErr.Expected)
	assert.Equal(t, []string{"cmd", "wrong"}, mErr.Received)
}

func TestEngine_ScenarioComplete(t *testing.T) {
	scn := buildScenario("test",
		leafStep([]string{"cmd"}, "done", 0),
	)
	eng := New(scn)
	ctx := context.Background()

	_, err := eng.Match(ctx, "cmd", nil)
	require.NoError(t, err)

	_, err = eng.Match(ctx, "cmd", nil)
	require.Error(t, err)

	var cErr *ScenarioCompleteError
	require.ErrorAs(t, err, &cErr)
	assert.Equal(t, 1, cErr.TotalSteps)
}

func TestEngine_ExitCode(t *testing.T) {
	scn := buildScenario("test",
		leafStep([]string{"cmd"}, "", 42),
	)
	eng := New(scn)

	result, err := eng.Match(context.Background(), "cmd", nil)
	require.NoError(t, err)
	assert.Equal(t, 42, result.ExitCode)
}

func TestEngine_Stderr(t *testing.T) {
	scn := buildScenario("test",
		scenario.StepElement{
			Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 1, Stderr: "error msg\n"},
			},
		},
	)
	eng := New(scn)

	result, err := eng.Match(context.Background(), "cmd", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.Equal(t, "error msg\n", result.Stderr)
}

func TestEngine_BudgetCalls(t *testing.T) {
	scn := buildScenario("budget",
		leafStepWithCalls([]string{"cmd", "repeat"}, "hit\n", 0, 1, 3),
		leafStep([]string{"cmd", "next"}, "done\n", 0),
	)
	eng := New(scn)
	ctx := context.Background()

	// Call 3 times (budget max=3)
	for i := 0; i < 3; i++ {
		r, err := eng.Match(ctx, "cmd", []string{"repeat"})
		require.NoError(t, err)
		assert.Equal(t, "hit\n", r.Stdout)
		assert.Equal(t, 0, r.StepIndex)
	}

	// 4th call should advance to next step
	r, err := eng.Match(ctx, "cmd", []string{"next"})
	require.NoError(t, err)
	assert.Equal(t, "done\n", r.Stdout)
	assert.Equal(t, 1, r.StepIndex)
}

func TestEngine_SoftAdvance(t *testing.T) {
	// Step 0: min=1, max=2. After 1 call, if next command doesn't match,
	// it should soft-advance to step 1.
	scn := buildScenario("soft",
		leafStepWithCalls([]string{"cmd", "a"}, "a\n", 0, 1, 2),
		leafStep([]string{"cmd", "b"}, "b\n", 0),
	)
	eng := New(scn)
	ctx := context.Background()

	// Call step 0 once (meets min)
	_, err := eng.Match(ctx, "cmd", []string{"a"})
	require.NoError(t, err)

	// Call step 1 directly — should soft-advance
	r, err := eng.Match(ctx, "cmd", []string{"b"})
	require.NoError(t, err)
	assert.Equal(t, "b\n", r.Stdout)
	assert.Equal(t, 1, r.StepIndex)
}

func TestEngine_UnorderedGroup(t *testing.T) {
	scn := buildScenario("group",
		groupStep("mygroup",
			leafStep([]string{"cmd", "a"}, "a\n", 0),
			leafStep([]string{"cmd", "b"}, "b\n", 0),
		),
	)
	eng := New(scn)
	ctx := context.Background()

	// Match out of order
	r1, err := eng.Match(ctx, "cmd", []string{"b"})
	require.NoError(t, err)
	assert.Equal(t, "b\n", r1.Stdout)

	r2, err := eng.Match(ctx, "cmd", []string{"a"})
	require.NoError(t, err)
	assert.Equal(t, "a\n", r2.Stdout)
}

func TestEngine_GroupMismatch(t *testing.T) {
	scn := buildScenario("group",
		groupStep("mygroup",
			leafStep([]string{"cmd", "a"}, "a", 0),
			leafStep([]string{"cmd", "b"}, "b", 0),
		),
	)
	eng := New(scn)

	_, err := eng.Match(context.Background(), "cmd", []string{"c"})
	require.Error(t, err)

	var gErr *GroupMismatchError
	require.ErrorAs(t, err, &gErr)
	assert.Equal(t, "mygroup", gErr.GroupName)
}

func TestEngine_CaptureChain(t *testing.T) {
	scn := buildScenario("captures",
		leafStepWithCapture([]string{"create"}, "created", 0, map[string]string{"id": "abc-123"}),
		scenario.StepElement{
			Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"show"}},
				Respond: scenario.Response{Exit: 0, Stdout: "id={{ .capture.id }}"},
			},
		},
	)
	eng := New(scn)
	ctx := context.Background()

	r1, err := eng.Match(ctx, "create", nil)
	require.NoError(t, err)
	assert.Equal(t, "abc-123", r1.Captures["id"])

	r2, err := eng.Match(ctx, "show", nil)
	require.NoError(t, err)
	assert.Equal(t, "id=abc-123", r2.Stdout)
}

func TestEngine_CaptureInGroup(t *testing.T) {
	scn := buildScenario("capture-group",
		leafStepWithCapture([]string{"setup"}, "ready", 0, map[string]string{"base": "base-1"}),
		groupStep("grp",
			leafStepWithCapture([]string{"cmd", "a"}, "a", 0, map[string]string{"val_a": "A"}),
			scenario.StepElement{
				Step: &scenario.Step{
					Match:   scenario.Match{Argv: []string{"cmd", "b"}},
					Respond: scenario.Response{Exit: 0, Stdout: "base={{ .capture.base }} a={{ .capture.val_a }}"},
				},
			},
		),
	)
	eng := New(scn)
	ctx := context.Background()

	_, err := eng.Match(ctx, "setup", nil)
	require.NoError(t, err)

	_, err = eng.Match(ctx, "cmd", []string{"a"})
	require.NoError(t, err)

	r, err := eng.Match(ctx, "cmd", []string{"b"})
	require.NoError(t, err)
	assert.Equal(t, "base=base-1 a=A", r.Stdout)
}

func TestEngine_WildcardMatching(t *testing.T) {
	scn := buildScenario("wildcard",
		leafStep([]string{"cmd", "{{ .any }}", "fixed"}, "matched\n", 0),
	)
	eng := New(scn)

	r, err := eng.Match(context.Background(), "cmd", []string{"anything-here", "fixed"})
	require.NoError(t, err)
	assert.Equal(t, "matched\n", r.Stdout)
}

func TestEngine_RegexMatching(t *testing.T) {
	scn := buildScenario("regex",
		leafStep([]string{"cmd", `{{ .regex "^v[0-9]+\.[0-9]+$" }}`}, "matched\n", 0),
	)
	eng := New(scn)

	r, err := eng.Match(context.Background(), "cmd", []string{"v1.23"})
	require.NoError(t, err)
	assert.Equal(t, "matched\n", r.Stdout)

	// Reset and try non-matching
	eng.Reset()
	_, err = eng.Match(context.Background(), "cmd", []string{"invalid"})
	require.Error(t, err)
}

func TestEngine_TemplateVars(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "vars",
			Vars: map[string]string{"region": "us-east-1"},
		},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 0, Stdout: "region={{ .region }}"},
			}},
		},
	}
	eng := New(scn)

	r, err := eng.Match(context.Background(), "cmd", nil)
	require.NoError(t, err)
	assert.Equal(t, "region=us-east-1", r.Stdout)
}

func TestEngine_WithVarsOverride(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "vars",
			Vars: map[string]string{"region": "us-east-1"},
		},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 0, Stdout: "region={{ .region }}"},
			}},
		},
	}
	eng := New(scn, WithVars(map[string]string{"region": "eu-west-1"}))

	r, err := eng.Match(context.Background(), "cmd", nil)
	require.NoError(t, err)
	assert.Equal(t, "region=eu-west-1", r.Stdout)
}

func TestEngine_WithEnvLookup(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "env",
			Vars: map[string]string{"region": "default"},
		},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 0, Stdout: "region={{ .region }}"},
			}},
		},
	}
	eng := New(scn, WithEnvLookup(func(key string) string {
		if key == "region" {
			return "from-env"
		}
		return ""
	}))

	r, err := eng.Match(context.Background(), "cmd", nil)
	require.NoError(t, err)
	assert.Equal(t, "region=from-env", r.Stdout)
}

func TestEngine_WithDenyEnvPatterns(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "deny",
			Vars: map[string]string{"SECRET_KEY": "default-secret"},
		},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 0, Stdout: "key={{ .SECRET_KEY }}"},
			}},
		},
	}
	eng := New(scn,
		WithEnvLookup(func(key string) string {
			if key == "SECRET_KEY" {
				return "env-secret"
			}
			return ""
		}),
		WithDenyEnvPatterns([]string{"SECRET_*"}),
	)

	r, err := eng.Match(context.Background(), "cmd", nil)
	require.NoError(t, err)
	// Env override denied → uses meta.vars value
	assert.Equal(t, "key=default-secret", r.Stdout)
}

func TestEngine_WithFileReader(t *testing.T) {
	scn := buildScenario("file",
		scenario.StepElement{
			Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 0, StdoutFile: "fixtures/output.txt"},
			},
		},
	)
	eng := New(scn, WithFileReader(func(path string) (string, error) {
		if path == "fixtures/output.txt" {
			return "file content\n", nil
		}
		return "", assert.AnError
	}))

	r, err := eng.Match(context.Background(), "cmd", nil)
	require.NoError(t, err)
	assert.Equal(t, "file content\n", r.Stdout)
}

func TestEngine_FileReaderNotConfigured(t *testing.T) {
	scn := buildScenario("file",
		scenario.StepElement{
			Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 0, StdoutFile: "fixtures/output.txt"},
			},
		},
	)
	eng := New(scn) // no file reader

	_, err := eng.Match(context.Background(), "cmd", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no file reader configured")
}

func TestEngine_WithMatchFunc(t *testing.T) {
	scn := buildScenario("custom",
		leafStep([]string{"cmd", "target"}, "matched\n", 0),
	)
	// Custom matcher that always matches
	eng := New(scn, WithMatchFunc(func(expected, received []string) bool {
		return true
	}))

	r, err := eng.Match(context.Background(), "anything", []string{"at", "all"})
	require.NoError(t, err)
	assert.Equal(t, "matched\n", r.Stdout)
}

func TestEngine_StdinMatch(t *testing.T) {
	scn := buildScenario("stdin",
		scenario.StepElement{
			Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}, Stdin: "expected input"},
				Respond: scenario.Response{Exit: 0, Stdout: "ok"},
			},
		},
	)
	eng := New(scn)
	ctx := context.Background()

	r, err := eng.MatchWithStdin(ctx, "cmd", nil, "expected input")
	require.NoError(t, err)
	assert.Equal(t, "ok", r.Stdout)
}

func TestEngine_StdinMismatch(t *testing.T) {
	scn := buildScenario("stdin",
		scenario.StepElement{
			Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}, Stdin: "expected"},
				Respond: scenario.Response{Exit: 0, Stdout: "ok"},
			},
		},
	)
	eng := New(scn)

	_, err := eng.MatchWithStdin(context.Background(), "cmd", nil, "wrong")
	require.Error(t, err)

	var sErr *StdinMismatchError
	require.ErrorAs(t, err, &sErr)
	assert.Equal(t, "expected", sErr.Expected)
	assert.Equal(t, "wrong", sErr.Received)
}

func TestEngine_Reset(t *testing.T) {
	scn := buildScenario("reset",
		leafStep([]string{"cmd"}, "output", 0),
	)
	eng := New(scn)
	ctx := context.Background()

	_, err := eng.Match(ctx, "cmd", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, eng.Remaining())

	eng.Reset()
	assert.Equal(t, 1, eng.Remaining())

	r, err := eng.Match(ctx, "cmd", nil)
	require.NoError(t, err)
	assert.Equal(t, "output", r.Stdout)
}

func TestEngine_StepCounts(t *testing.T) {
	scn := buildScenario("counts",
		leafStepWithCalls([]string{"cmd"}, "hit", 0, 1, 3),
	)
	eng := New(scn)
	ctx := context.Background()

	_, _ = eng.Match(ctx, "cmd", nil)
	_, _ = eng.Match(ctx, "cmd", nil)

	counts := eng.StepCounts()
	assert.Equal(t, []int{2}, counts)
}

func TestEngine_ThreadSafety(t *testing.T) {
	scn := buildScenario("concurrent",
		leafStepWithCalls([]string{"cmd"}, "hit", 0, 1, 100),
	)
	eng := New(scn)
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := eng.Match(ctx, "cmd", nil)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("unexpected error in concurrent access: %v", err)
	}

	counts := eng.StepCounts()
	assert.Equal(t, 50, counts[0])
}

func TestEngine_GroupBudget(t *testing.T) {
	scn := buildScenario("group-budget",
		groupStep("grp",
			leafStepWithCalls([]string{"cmd", "a"}, "a", 0, 1, 2),
			leafStepWithCalls([]string{"cmd", "b"}, "b", 0, 1, 2),
		),
		leafStep([]string{"cmd", "after"}, "after", 0),
	)
	eng := New(scn)
	ctx := context.Background()

	// Exhaust budget for both group steps
	_, err := eng.Match(ctx, "cmd", []string{"a"})
	require.NoError(t, err)
	_, err = eng.Match(ctx, "cmd", []string{"a"})
	require.NoError(t, err)
	_, err = eng.Match(ctx, "cmd", []string{"b"})
	require.NoError(t, err)
	_, err = eng.Match(ctx, "cmd", []string{"b"})
	require.NoError(t, err)

	// Group exhausted → next step
	r, err := eng.Match(ctx, "cmd", []string{"after"})
	require.NoError(t, err)
	assert.Equal(t, "after", r.Stdout)
}

func TestEngine_GroupSoftAdvancePastGroup(t *testing.T) {
	// Group with mins met, then a step after group
	scn := buildScenario("group-soft",
		groupStep("grp",
			leafStep([]string{"cmd", "a"}, "a", 0),
		),
		leafStep([]string{"cmd", "after"}, "after", 0),
	)
	eng := New(scn)
	ctx := context.Background()

	// Satisfy group min
	_, err := eng.Match(ctx, "cmd", []string{"a"})
	require.NoError(t, err)

	// Non-matching command in group → mins met → soft-advance past group
	r, err := eng.Match(ctx, "cmd", []string{"after"})
	require.NoError(t, err)
	assert.Equal(t, "after", r.Stdout)
}

func TestNormalizeStdin(t *testing.T) {
	assert.Equal(t, "hello", normalizeStdin("hello\n"))
	assert.Equal(t, "hello", normalizeStdin("hello\r\n"))
	assert.Equal(t, "hello\nworld", normalizeStdin("hello\r\nworld\n"))
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"AWS_*", "AWS_SECRET", true},
		{"AWS_*", "GCP_KEY", false},
		{"SECRET_?", "SECRET_X", true},
		{"SECRET_?", "SECRET_XY", false},
		{"*", "anything", true},
		{"exact", "exact", true},
		{"exact", "nope", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, globMatch(tt.pattern, tt.name))
		})
	}
}
