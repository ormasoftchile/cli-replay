// Package integration_test provides end-to-end integration tests for the
// cli-replay public API surface. These tests exercise the full pipeline:
// Load scenario → Match commands → Build verification results.
//
// Placed in a standalone test package to avoid import cycles between
// pkg/replay, pkg/verify, and internal/runner.
package integration_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ormasoftchile/cli-replay/internal/runner"
	"github.com/ormasoftchile/cli-replay/pkg/matcher"
	"github.com/ormasoftchile/cli-replay/pkg/scenario"
	"github.com/ormasoftchile/cli-replay/pkg/verify"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Integration tests: Load → Match → Verify pipeline
//
// These tests exercise the full public API surface from a consumer's
// perspective (e.g., gert importing cli-replay as a library). They validate
// the end-to-end contract: load YAML → match commands → build verification
// results.
// ═══════════════════════════════════════════════════════════════════════════════

// --- Helper: simulate ordered matching ---------------------------------------

// simulateOrderedReplay walks through a flat step list in order, matching each
// received command against the current step with budget-aware soft-advance.
func simulateOrderedReplay(steps []scenario.Step, commands [][]string) *runner.State {
	state := runner.NewState("test.yaml", "hash", len(steps))
	for _, argv := range commands {
		for i := state.CurrentStep; i < len(steps); i++ {
			bounds := steps[i].EffectiveCalls()
			if state.StepBudgetRemaining(i, bounds.Max) <= 0 {
				state.CurrentStep = i + 1
				continue
			}
			if matcher.ArgvMatch(steps[i].Match.Argv, argv) {
				state.IncrementStep(i)
				if state.StepBudgetRemaining(i, bounds.Max) <= 0 {
					state.CurrentStep = i + 1
				}
				break
			}
			// If min met, soft-advance
			if state.StepCounts[i] >= bounds.Min && i+1 < len(steps) {
				state.CurrentStep = i + 1
				if matcher.ArgvMatch(steps[i+1].Match.Argv, argv) {
					state.IncrementStep(i + 1)
					bounds2 := steps[i+1].EffectiveCalls()
					if state.StepBudgetRemaining(i+1, bounds2.Max) <= 0 {
						state.CurrentStep = i + 2
					}
					break
				}
			}
			break
		}
	}
	return state
}

// simulateUnorderedGroupReplay matches commands against a group of steps in
// any order, respecting call budgets.
func simulateUnorderedGroupReplay(steps []scenario.Step, commands [][]string) *runner.State {
	state := runner.NewState("test.yaml", "hash", len(steps))
	for _, argv := range commands {
		for i := 0; i < len(steps); i++ {
			bounds := steps[i].EffectiveCalls()
			if state.StepBudgetRemaining(i, bounds.Max) <= 0 {
				continue
			}
			if matcher.ArgvMatch(steps[i].Match.Argv, argv) {
				state.IncrementStep(i)
				break
			}
		}
	}
	return state
}

// --- Integration: Load YAML → Match → Verify (ordered) ----------------------

func TestIntegration_LoadMatchVerify_OrderedScenario(t *testing.T) {
	yamlContent := `
meta:
  name: deploy-pipeline
steps:
  - match:
      argv: ["kubectl", "get", "pods", "-n", "default"]
    respond:
      exit: 0
      stdout: "web-0 Running"
  - match:
      argv: ["kubectl", "apply", "-f", "deploy.yaml"]
    respond:
      exit: 0
      stdout: "deployment configured"
  - match:
      argv: ["kubectl", "rollout", "status", "deployment/web"]
    respond:
      exit: 0
      stdout: "successfully rolled out"
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()
	require.Len(t, flat, 3)

	commands := [][]string{
		{"kubectl", "get", "pods", "-n", "default"},
		{"kubectl", "apply", "-f", "deploy.yaml"},
		{"kubectl", "rollout", "status", "deployment/web"},
	}

	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "test-session", flat, state.StepCounts, nil)

	assert.True(t, result.Passed)
	assert.Equal(t, 3, result.TotalSteps)
	assert.Equal(t, 3, result.ConsumedSteps)
	assert.Equal(t, "deploy-pipeline", result.Scenario)
	for _, s := range result.Steps {
		assert.True(t, s.Passed, "step %d should pass", s.Index)
	}
}

// --- Integration: Wildcard matching -----------------------------------------

func TestIntegration_WildcardMatching(t *testing.T) {
	yamlContent := `
meta:
  name: wildcard-test
steps:
  - match:
      argv: ["az", "group", "list", "--subscription", "{{ .any }}"]
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()
	commands := [][]string{
		{"az", "group", "list", "--subscription", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
	}

	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
	assert.True(t, result.Passed)
}

func TestIntegration_WildcardNoMatch_DifferentLength(t *testing.T) {
	yamlContent := `
meta:
  name: wildcard-length
steps:
  - match:
      argv: ["cmd", "{{ .any }}", "flag"]
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()
	// Wrong length — should NOT match
	commands := [][]string{{"cmd", "value"}}

	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
	assert.False(t, result.Passed, "length mismatch should fail")
}

// --- Integration: Regex matching --------------------------------------------

func TestIntegration_RegexMatching(t *testing.T) {
	yamlContent := `
meta:
  name: regex-test
steps:
  - match:
      argv: ["az", "--subscription", '{{ .regex "^[0-9a-f-]{36}$" }}']
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()

	t.Run("valid UUID matches", func(t *testing.T) {
		commands := [][]string{
			{"az", "--subscription", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
		}
		state := simulateOrderedReplay(flat, commands)
		result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
		assert.True(t, result.Passed)
	})

	t.Run("non-UUID rejected", func(t *testing.T) {
		commands := [][]string{
			{"az", "--subscription", "not-a-uuid"},
		}
		state := simulateOrderedReplay(flat, commands)
		result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
		assert.False(t, result.Passed)
	})
}

// --- Integration: Budget-aware ordered matching -----------------------------

func TestIntegration_BudgetAware_OrderedReplay(t *testing.T) {
	yamlContent := `
meta:
  name: budget-ordered
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    calls:
      min: 1
      max: 3
    respond:
      exit: 0
  - match:
      argv: ["kubectl", "apply", "-f", "deploy.yaml"]
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()

	t.Run("call step multiple times within budget", func(t *testing.T) {
		commands := [][]string{
			{"kubectl", "get", "pods"},
			{"kubectl", "get", "pods"},
			{"kubectl", "apply", "-f", "deploy.yaml"},
		}
		state := simulateOrderedReplay(flat, commands)
		result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
		assert.True(t, result.Passed)
		assert.Equal(t, 2, result.Steps[0].CallCount)
	})

	t.Run("exhaust budget then advance", func(t *testing.T) {
		commands := [][]string{
			{"kubectl", "get", "pods"},
			{"kubectl", "get", "pods"},
			{"kubectl", "get", "pods"},
			{"kubectl", "apply", "-f", "deploy.yaml"},
		}
		state := simulateOrderedReplay(flat, commands)
		result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
		assert.True(t, result.Passed)
		assert.Equal(t, 3, result.Steps[0].CallCount)
		assert.Equal(t, 1, result.Steps[1].CallCount)
	})
}

// --- Integration: Budget-aware unordered matching ---------------------------

func TestIntegration_BudgetAware_UnorderedReplay(t *testing.T) {
	steps := []scenario.Step{
		{
			Match:   scenario.Match{Argv: []string{"kubectl", "get", "pods"}},
			Respond: scenario.Response{Exit: 0},
			Calls:   &scenario.CallBounds{Min: 1, Max: 2},
		},
		{
			Match:   scenario.Match{Argv: []string{"kubectl", "get", "svc"}},
			Respond: scenario.Response{Exit: 0},
		},
		{
			Match:   scenario.Match{Argv: []string{"kubectl", "get", "nodes"}},
			Respond: scenario.Response{Exit: 0},
			Calls:   &scenario.CallBounds{Min: 0, Max: 1},
		},
	}

	t.Run("all matched in arbitrary order", func(t *testing.T) {
		commands := [][]string{
			{"kubectl", "get", "svc"},
			{"kubectl", "get", "nodes"},
			{"kubectl", "get", "pods"},
		}
		state := simulateUnorderedGroupReplay(steps, commands)
		result := verify.BuildResult("unordered-test", "default", steps, state.StepCounts, nil)
		assert.True(t, result.Passed)
		assert.Equal(t, 3, result.ConsumedSteps)
	})

	t.Run("optional step uncalled is OK", func(t *testing.T) {
		commands := [][]string{
			{"kubectl", "get", "pods"},
			{"kubectl", "get", "svc"},
		}
		state := simulateUnorderedGroupReplay(steps, commands)
		result := verify.BuildResult("unordered-test", "default", steps, state.StepCounts, nil)
		assert.True(t, result.Passed, "optional step (min=0) uncalled should pass")
	})

	t.Run("multi-call within budget", func(t *testing.T) {
		commands := [][]string{
			{"kubectl", "get", "pods"},
			{"kubectl", "get", "pods"},
			{"kubectl", "get", "svc"},
		}
		state := simulateUnorderedGroupReplay(steps, commands)
		result := verify.BuildResult("unordered-test", "default", steps, state.StepCounts, nil)
		assert.True(t, result.Passed)
		assert.Equal(t, 2, result.Steps[0].CallCount)
	})

	t.Run("budget exhausted prevents further matching", func(t *testing.T) {
		commands := [][]string{
			{"kubectl", "get", "pods"},
			{"kubectl", "get", "pods"},
			{"kubectl", "get", "pods"}, // exceeds max=2, should be ignored
			{"kubectl", "get", "svc"},
		}
		state := simulateUnorderedGroupReplay(steps, commands)
		assert.Equal(t, 2, state.StepCounts[0], "should not exceed max budget")
	})
}

// --- Edge case: empty scenario -----------------------------------------------

func TestIntegration_EmptyScenario_Rejected(t *testing.T) {
	yamlContent := `
meta:
  name: empty
steps: []
`
	_, err := scenario.Load(strings.NewReader(yamlContent))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "steps must contain at least one step")
}

// --- Edge case: no match found -----------------------------------------------

func TestIntegration_NoMatchFound(t *testing.T) {
	yamlContent := `
meta:
  name: no-match
steps:
  - match:
      argv: ["expected-cmd"]
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()
	commands := [][]string{{"different-cmd", "arg"}}
	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)

	assert.False(t, result.Passed, "unmatched command should leave scenario incomplete")
	assert.Equal(t, 0, result.ConsumedSteps)
}

// --- Edge case: nil state → error result ------------------------------------

func TestIntegration_NilState_ErrorResult(t *testing.T) {
	steps := []scenario.Step{
		{Match: scenario.Match{Argv: []string{"cmd"}}, Respond: scenario.Response{Exit: 0}},
	}
	result := verify.BuildResult("test", "default", steps, nil, nil)
	assert.False(t, result.Passed)
	assert.Equal(t, "no state found", result.Error)
	assert.Equal(t, 0, result.TotalSteps)
}

// --- Integration: scenario with groups → verify with group metadata ----------

func TestIntegration_GroupScenario_LoadAndVerify(t *testing.T) {
	yamlContent := `
meta:
  name: grouped-deploy
steps:
  - match:
      argv: ["setup"]
    respond:
      exit: 0
  - group:
      mode: unordered
      name: pre-checks
      steps:
        - match:
            argv: ["check", "dns"]
          respond:
            exit: 0
        - match:
            argv: ["check", "api"]
          respond:
            exit: 0
  - match:
      argv: ["deploy"]
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()
	require.Len(t, flat, 4)

	groupRanges := scn.GroupRanges()
	require.Len(t, groupRanges, 1)
	assert.Equal(t, "pre-checks", groupRanges[0].Name)
	assert.Equal(t, 1, groupRanges[0].Start)
	assert.Equal(t, 3, groupRanges[0].End)

	// Simulate: setup → check api → check dns → deploy (group in reverse order)
	state := runner.NewState("test.yaml", "hash", len(flat))
	state.IncrementStep(0) // setup
	state.IncrementStep(2) // check api (out of order)
	state.IncrementStep(1) // check dns (out of order)
	state.IncrementStep(3) // deploy

	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, groupRanges)
	assert.True(t, result.Passed)
	assert.Equal(t, 4, result.ConsumedSteps)

	// Verify group metadata in results
	assert.Empty(t, result.Steps[0].Group)
	assert.Equal(t, "pre-checks", result.Steps[1].Group)
	assert.Equal(t, "pre-checks", result.Steps[2].Group)
	assert.Empty(t, result.Steps[3].Group)
}

// --- Integration: Load real testdata fixtures --------------------------------

func TestIntegration_LoadCallBoundsFixture(t *testing.T) {
	scn, err := scenario.LoadFile("../../testdata/scenarios/call_bounds.yaml")
	require.NoError(t, err)

	assert.Equal(t, "call-bounds-demo", scn.Meta.Name)
	flat := scn.FlatSteps()
	require.Len(t, flat, 3)

	// Step 0: min=1 max=5
	assert.NotNil(t, flat[0].Calls)
	assert.Equal(t, 1, flat[0].Calls.Min)
	assert.Equal(t, 5, flat[0].Calls.Max)

	// Step 1: default bounds
	assert.Nil(t, flat[1].Calls)
	bounds := flat[1].EffectiveCalls()
	assert.Equal(t, 1, bounds.Min)
	assert.Equal(t, 1, bounds.Max)

	// Step 2: min=1 max=3
	assert.NotNil(t, flat[2].Calls)
	assert.Equal(t, 1, flat[2].Calls.Min)
	assert.Equal(t, 3, flat[2].Calls.Max)
}

func TestIntegration_LoadMultiStepFixture(t *testing.T) {
	scn, err := scenario.LoadFile("../../testdata/scenarios/multi_step.yaml")
	require.NoError(t, err)

	assert.Equal(t, "multi-step-test", scn.Meta.Name)
	assert.Equal(t, "production", scn.Meta.Vars["namespace"])
	flat := scn.FlatSteps()
	require.Len(t, flat, 3)

	commands := [][]string{
		{"kubectl", "get", "pods", "-n", "production"},
		{"kubectl", "rollout", "restart", "deployment/web"},
		{"kubectl", "get", "pods", "-n", "production"},
	}
	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
	assert.True(t, result.Passed)
}

func TestIntegration_LoadSingleStepFixture(t *testing.T) {
	scn, err := scenario.LoadFile("../../testdata/scenarios/single_step.yaml")
	require.NoError(t, err)

	assert.Equal(t, "single-step-test", scn.Meta.Name)
	flat := scn.FlatSteps()
	require.Len(t, flat, 1)

	commands := [][]string{{"kubectl", "get", "pods"}}
	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
	assert.True(t, result.Passed)
}

func TestIntegration_LoadCaptureGroupFixture(t *testing.T) {
	scn, err := scenario.LoadFile("../../testdata/scenarios/capture_group.yaml")
	require.NoError(t, err)

	assert.Equal(t, "capture-group", scn.Meta.Name)
	flat := scn.FlatSteps()
	require.Len(t, flat, 4)

	groups := scn.GroupRanges()
	require.Len(t, groups, 1)
	assert.Equal(t, "monitoring", groups[0].Name)
	assert.Equal(t, 1, groups[0].Start)
	assert.Equal(t, 3, groups[0].End)
}

// --- Integration: Mixed literal/wildcard/regex in one scenario ---------------

func TestIntegration_MixedMatchTypes(t *testing.T) {
	yamlContent := `
meta:
  name: mixed-match
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
  - match:
      argv: ["az", "{{ .any }}", "list"]
    respond:
      exit: 0
  - match:
      argv: ["docker", '{{ .regex "^(push|pull)$" }}', '{{ .regex "^[a-z]+/[a-z]+:" }}']
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()

	commands := [][]string{
		{"kubectl", "get", "pods"},
		{"az", "group", "list"},
		{"docker", "push", "myrepo/myimage:latest"},
	}

	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
	assert.True(t, result.Passed)
	assert.Equal(t, 3, result.ConsumedSteps)
}

// --- Verification JSON round-trip for full pipeline --------------------------

func TestIntegration_FullPipeline_JSONOutput(t *testing.T) {
	yamlContent := `
meta:
  name: json-pipeline
steps:
  - match:
      argv: ["git", "status"]
    respond:
      exit: 0
  - match:
      argv: ["git", "push"]
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()
	commands := [][]string{
		{"git", "status"},
		{"git", "push"},
	}
	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)

	var buf strings.Builder
	require.NoError(t, verify.FormatJSON(&buf, result))

	assert.Contains(t, buf.String(), `"passed":true`)
	assert.Contains(t, buf.String(), `"scenario":"json-pipeline"`)
	assert.Contains(t, buf.String(), `"total_steps":2`)
}

// --- Matcher diagnostics: ElementMatchDetail for mismatch reporting ----------

func TestIntegration_MatchDiagnostics(t *testing.T) {
	expected := []string{"kubectl", "get", "pods", "-n", "default"}
	received := []string{"kubectl", "get", "pods", "-n", "production"}

	assert.False(t, matcher.ArgvMatch(expected, received))

	// Find the divergence point
	for i := range expected {
		d := matcher.ElementMatchDetail(expected[i], received[i])
		if !d.Matched {
			assert.Equal(t, 4, i, "divergence at position 4 (namespace value)")
			assert.Equal(t, "literal", d.Kind)
			assert.Contains(t, d.FailReason, `expected "default"`)
			assert.Contains(t, d.FailReason, `got "production"`)
			break
		}
	}
}

// --- Soft-advance: ordered step with min met skips to next -------------------

func TestIntegration_SoftAdvance_MinMet(t *testing.T) {
	yamlContent := `
meta:
  name: soft-advance
steps:
  - match:
      argv: ["health-check"]
    calls:
      min: 1
      max: 5
    respond:
      exit: 0
  - match:
      argv: ["deploy"]
    respond:
      exit: 0
`
	scn, err := scenario.Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	flat := scn.FlatSteps()

	// Call health-check once (min=1 met), then deploy (should soft-advance)
	commands := [][]string{
		{"health-check"},
		{"deploy"},
	}
	state := simulateOrderedReplay(flat, commands)
	result := verify.BuildResult(scn.Meta.Name, "default", flat, state.StepCounts, nil)
	assert.True(t, result.Passed)
	assert.Equal(t, 1, result.Steps[0].CallCount)
	assert.Equal(t, 1, result.Steps[1].CallCount)
}

// --- Full pipeline with security config loaded from fixture ------------------

func TestIntegration_LoadSecurityFixture(t *testing.T) {
	scn, err := scenario.LoadFile("../../testdata/scenarios/security_allowlist.yaml")
	require.NoError(t, err)

	assert.Equal(t, "security-allowlist", scn.Meta.Name)
	require.NotNil(t, scn.Meta.Security)
	assert.Equal(t, []string{"kubectl", "az"}, scn.Meta.Security.AllowedCommands)
}

func TestIntegration_LoadDenyEnvVarsFixture(t *testing.T) {
	scn, err := scenario.LoadFile("../../testdata/scenarios/deny_env_vars.yaml")
	require.NoError(t, err)

	assert.Equal(t, "deny-env-vars-demo", scn.Meta.Name)
	require.NotNil(t, scn.Meta.Security)
	assert.Equal(t, []string{"AWS_*", "SECRET_*", "TOKEN"}, scn.Meta.Security.DenyEnvVars)
}

// --- Capture chain fixture loaded and validated ------------------------------

func TestIntegration_LoadCaptureChainFixture(t *testing.T) {
	scn, err := scenario.LoadFile("../../testdata/scenarios/capture_chain.yaml")
	require.NoError(t, err)

	assert.Equal(t, "capture-chain", scn.Meta.Name)
	flat := scn.FlatSteps()
	require.Len(t, flat, 3)

	// Step 0 captures rg_id
	assert.Contains(t, flat[0].Respond.Capture, "rg_id")
	// Step 1 captures vm_id
	assert.Contains(t, flat[1].Respond.Capture, "vm_id")
	// Step 2 references both captures in stdout
	assert.Contains(t, flat[2].Respond.Stdout, "capture.rg_id")
	assert.Contains(t, flat[2].Respond.Stdout, "capture.vm_id")
}

// --- Session TTL fixture loaded and validated --------------------------------

func TestIntegration_LoadSessionTTLFixture(t *testing.T) {
	scn, err := scenario.LoadFile("../../testdata/scenarios/session_ttl.yaml")
	require.NoError(t, err)

	assert.Equal(t, "session-ttl-demo", scn.Meta.Name)
	require.NotNil(t, scn.Meta.Session)
	assert.Equal(t, "10m", scn.Meta.Session.TTL)
}
