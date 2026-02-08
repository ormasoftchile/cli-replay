package matcher

import (
	"fmt"
	"testing"
)

// generateSteps builds n step patterns with varying argv lengths for
// realistic benchmark scenarios.
func generateSteps(n int) [][]string {
	steps := make([][]string, n)
	for i := range steps {
		steps[i] = []string{
			"kubectl",
			"get",
			fmt.Sprintf("resource-%d", i),
			"-n",
			fmt.Sprintf("namespace-%d", i%10),
		}
	}
	return steps
}

// BenchmarkArgvMatch benchmarks ArgvMatch against scenarios with 100 and 500
// steps. For each sub-benchmark, a worst-case linear scan is simulated: the
// matching step is the last one in the list.
func BenchmarkArgvMatch(b *testing.B) {
	for _, n := range []int{100, 500} {
		steps := generateSteps(n)
		// The "received" argv matches the last step (worst-case scan)
		received := steps[n-1]

		b.Run(fmt.Sprintf("steps=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for _, expected := range steps {
					if ArgvMatch(expected, received) {
						break
					}
				}
			}
		})
	}
}

// BenchmarkGroupMatch_50 benchmarks matching within a 50-step unordered group.
// Simulates the linear scan performed when matching inside a group — each
// iteration scans all 50 steps to find the matching one at the end.
func BenchmarkGroupMatch_50(b *testing.B) {
	steps := generateSteps(50)
	received := steps[49] // worst case: match is last

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, expected := range steps {
			if ArgvMatch(expected, received) {
				break
			}
		}
	}
}

// BenchmarkRegexPathological exercises a known-pathological regex pattern
// against a non-matching input to demonstrate that Go's RE2 engine handles
// it safely in linear time. In PCRE engines this pattern causes exponential
// backtracking; in RE2 it completes in microseconds.
//
// Pattern: ^(a+)+$  Input: "aaa...ab" (50 chars)
// See: SECURITY.md "Regex Safety (ReDoS Prevention)" section.
func BenchmarkRegexPathological(b *testing.B) {
	// 49 'a's followed by 'b' — guarantees no match.
	input := make([]byte, 50)
	for i := 0; i < 49; i++ {
		input[i] = 'a'
	}
	input[49] = 'b'
	inputStr := string(input)

	// The pattern uses grouping + repetition that triggers exponential
	// backtracking in PCRE but is O(n) in RE2.
	pattern := []string{"{{ .regex `^(a+)+$` }}"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ArgvMatch(pattern, []string{inputStr})
	}
}
