package runner

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/cli-replay/cli-replay/internal/scenario"
)

// BenchmarkStateRoundTrip measures WriteState → ReadState round-trip cost
// with parametric step counts.
func BenchmarkStateRoundTrip(b *testing.B) {
	for _, n := range []int{100, 500} {
		b.Run(fmt.Sprintf("steps=%d", n), func(b *testing.B) {
			dir := b.TempDir()
			path := dir + "/bench.state"

			state := NewState("/tmp/scenario.yaml", "hash123", n)
			// Populate step counts to simulate a partially-consumed scenario
			for i := 0; i < n; i++ {
				state.StepCounts[i] = i % 3
			}
			// Add some captures
			state.Captures["rg_id"] = "/subscriptions/abc/rg"
			state.Captures["vm_id"] = "/subscriptions/abc/vm"

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := WriteState(path, state); err != nil {
					b.Fatal(err)
				}
				if _, err := ReadState(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkReplayOrchestration_100 benchmarks the core replay matching engine
// with a 100-step synthetic scenario. This measures the overhead of step
// matching, state advancement, and response rendering — without actual file
// I/O for the scenario load (pre-built in memory).
func BenchmarkReplayOrchestration_100(b *testing.B) {
	const n = 100

	// Build a synthetic scenario in memory
	elements := make([]scenario.StepElement, n)
	for i := range elements {
		elements[i] = scenario.StepElement{
			Step: &scenario.Step{
				Match: scenario.Match{
					Argv: []string{"kubectl", "get", fmt.Sprintf("resource-%d", i)},
				},
				Respond: scenario.Response{
					Exit:   0,
					Stdout: fmt.Sprintf("output line %d\n", i),
				},
			},
		}
	}

	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "bench-100",
			Vars: map[string]string{"cluster": "prod"},
		},
		Steps: elements,
	}

	flatSteps := scn.FlatSteps()
	groupRanges := scn.GroupRanges()

	var stdout, stderr bytes.Buffer

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create new state each iteration (simulates fresh replay session)
		state := NewState("/tmp/bench.yaml", "hash", n)

		// Simulate matching all 100 steps in order
		for stepIdx := 0; stepIdx < n; stepIdx++ {
			argv := flatSteps[stepIdx].Match.Argv
			step := &flatSteps[stepIdx]

			// Core matching (simplified — no file I/O)
			matched := false
			for j := state.CurrentStep; j < len(flatSteps); j++ {
				if argvEqual(flatSteps[j].Match.Argv, argv) {
					state.IncrementStep(j)
					bounds := flatSteps[j].EffectiveCalls()
					if state.StepBudgetRemaining(j, bounds.Max) <= 0 {
						state.CurrentStep = j + 1
					}
					matched = true
					break
				}
			}
			if !matched {
				b.Fatalf("step %d did not match", stepIdx)
			}

			// Response rendering
			stdout.Reset()
			stderr.Reset()
			ReplayResponseWithTemplate(step, scn, "/tmp/scenario.yaml", state.Captures, &stdout, &stderr)
		}

		// Verify all mins met
		if !state.AllStepsMetMin(flatSteps) {
			b.Fatal("not all steps met min")
		}
		_ = groupRanges // used in real code, kept for API parity
	}
}

// argvEqual is a simplified argv comparison for benchmarking (no wildcards/regex).
func argvEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
