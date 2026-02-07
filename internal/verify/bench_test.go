package verify

import (
	"bytes"
	"testing"
	"time"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
)

// benchResult constructs a representative VerifyResult for benchmarking.
// It simulates a 10-step scenario with groups, matching real-world usage.
func benchResult() *VerifyResult {
	steps := make([]scenario.Step, 10)
	for i := range steps {
		steps[i] = scenario.Step{
			Match: scenario.Match{
				Argv: []string{"kubectl", "get", "pods", "-n", "production"},
			},
		}
	}

	state := runner.NewState("/tmp/bench.yaml", "abc123", 10)
	state.StepCounts = make([]int, 10)
	for i := range state.StepCounts {
		state.StepCounts[i] = 1
	}

	groupRanges := []scenario.GroupRange{
		{Start: 2, End: 5, Name: "pre-flight", TopIndex: 1},
		{Start: 7, End: 9, Name: "cleanup", TopIndex: 5},
	}

	return BuildResult("bench-scenario", "default", steps, state, groupRanges)
}

// BenchmarkFormatJSON measures JSON formatting overhead.
func BenchmarkFormatJSON(b *testing.B) {
	result := benchResult()
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = FormatJSON(&buf, result)
	}
}

// BenchmarkFormatJUnit measures JUnit XML formatting overhead.
func BenchmarkFormatJUnit(b *testing.B) {
	result := benchResult()
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = FormatJUnit(&buf, result, "/tmp/bench.yaml", ts)
	}
}

// BenchmarkBuildResult measures the cost of building the result struct itself.
func BenchmarkBuildResult(b *testing.B) {
	steps := make([]scenario.Step, 10)
	for i := range steps {
		steps[i] = scenario.Step{
			Match: scenario.Match{
				Argv: []string{"kubectl", "get", "pods", "-n", "production"},
			},
		}
	}

	state := runner.NewState("/tmp/bench.yaml", "abc123", 10)
	state.StepCounts = make([]int, 10)
	for i := range state.StepCounts {
		state.StepCounts[i] = 1
	}

	groupRanges := []scenario.GroupRange{
		{Start: 2, End: 5, Name: "pre-flight", TopIndex: 1},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildResult("bench-scenario", "default", steps, state, groupRanges)
	}
}

// TestFormatOverheadUnder5ms verifies SC-006: JSON/JUnit overhead < 5ms.
// Runs each formatter 1000 times and checks average per-call duration.
func TestFormatOverheadUnder5ms(t *testing.T) {
	result := benchResult()
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	const iterations = 1000

	// Warm up
	for i := 0; i < 10; i++ {
		buf.Reset()
		_ = FormatJSON(&buf, result)
		buf.Reset()
		_ = FormatJUnit(&buf, result, "/tmp/bench.yaml", ts)
	}

	// Measure JSON
	start := time.Now()
	for i := 0; i < iterations; i++ {
		buf.Reset()
		_ = FormatJSON(&buf, result)
	}
	jsonAvg := time.Since(start) / iterations

	// Measure JUnit
	start = time.Now()
	for i := 0; i < iterations; i++ {
		buf.Reset()
		_ = FormatJUnit(&buf, result, "/tmp/bench.yaml", ts)
	}
	junitAvg := time.Since(start) / iterations

	t.Logf("JSON  avg: %v per call (%d iterations)", jsonAvg, iterations)
	t.Logf("JUnit avg: %v per call (%d iterations)", junitAvg, iterations)

	const maxOverhead = 5 * time.Millisecond
	if jsonAvg > maxOverhead {
		t.Errorf("JSON format overhead %v exceeds SC-006 limit of %v", jsonAvg, maxOverhead)
	}
	if junitAvg > maxOverhead {
		t.Errorf("JUnit format overhead %v exceeds SC-006 limit of %v", junitAvg, maxOverhead)
	}
}
