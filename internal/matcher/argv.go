// Package matcher provides functions for matching CLI arguments against scenarios.
package matcher

// ArgvMatch performs strict comparison of two argument vectors.
// Returns true if both slices have the same length and all elements match exactly.
func ArgvMatch(expected, received []string) bool {
	if len(expected) != len(received) {
		return false
	}
	for i := range expected {
		if expected[i] != received[i] {
			return false
		}
	}
	return true
}
