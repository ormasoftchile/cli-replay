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
	return benchResultN(10)
}

// benchResultN constructs a parametric VerifyResult with n steps and
// proportional groups. For n >= 10, two groups are created covering ~30%
// and ~20% of the steps respectively.
func benchResultN(n int) *VerifyResult {
	steps := make([]scenario.Step, n)
	for i := range steps {
		steps[i] = scenario.Step{
			Match: scenario.Match{
				Argv: []string{"kubectl", "get", "pods", "-n", "production"},
			},
		}
	}

	state := runner.NewState("/tmp/bench.yaml", "abc123", n)
	state.StepCounts = make([]int, n)
	for i := range state.StepCounts {
		state.StepCounts[i] = 1
	}

	// Create groups proportional to step count
	var groupRanges []scenario.GroupRange
	if n >= 10 {
		g1Start := n / 5       // 20% mark
		g1End := g1Start + n/3 // ~33% of steps
		if g1End > n {
			g1End = n
		}
		groupRanges = append(groupRanges, scenario.GroupRange{
			Start: g1Start, End: g1End, Name: "pre-flight", TopIndex: 1,
		})

		g2Start := n * 7 / 10 // 70% mark
		g2End := g2Start + n/5
		if g2End > n {
			g2End = n
		}
		if g2Start < g1End {
			g2Start = g1End + 1
		}
		if g2Start < n && g2End > g2Start {
			groupRanges = append(groupRanges, scenario.GroupRange{
				Start: g2Start, End: g2End, Name: "cleanup", TopIndex: 5,
			})
		}
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

// BenchmarkFormatJSON_200 measures JSON formatting overhead at 200 steps.
func BenchmarkFormatJSON_200(b *testing.B) {
	result := benchResultN(200)
	var buf bytes.Buffer

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = FormatJSON(&buf, result)
	}
}

// BenchmarkFormatJUnit_200 measures JUnit XML formatting overhead at 200 steps.
func BenchmarkFormatJUnit_200(b *testing.B) {
	result := benchResultN(200)
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = FormatJUnit(&buf, result, "/tmp/bench.yaml", ts)
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
