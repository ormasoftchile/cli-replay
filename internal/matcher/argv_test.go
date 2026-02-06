// Package matcher provides functions for matching CLI arguments against scenarios.
package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:funlen // Table-driven test with comprehensive test cases
func TestArgvMatch(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		received []string
		want     bool
	}{
		{
			name:     "exact match single element",
			expected: []string{"cmd"},
			received: []string{"cmd"},
			want:     true,
		},
		{
			name:     "exact match multiple elements",
			expected: []string{"kubectl", "get", "pods"},
			received: []string{"kubectl", "get", "pods"},
			want:     true,
		},
		{
			name:     "exact match with flags",
			expected: []string{"kubectl", "get", "pods", "-n", "default", "-o", "json"},
			received: []string{"kubectl", "get", "pods", "-n", "default", "-o", "json"},
			want:     true,
		},
		{
			name:     "different command",
			expected: []string{"kubectl", "get", "pods"},
			received: []string{"kubectl", "get", "services"},
			want:     false,
		},
		{
			name:     "different length - expected longer",
			expected: []string{"kubectl", "get", "pods", "-n", "default"},
			received: []string{"kubectl", "get", "pods"},
			want:     false,
		},
		{
			name:     "different length - received longer",
			expected: []string{"kubectl", "get", "pods"},
			received: []string{"kubectl", "get", "pods", "-n", "default"},
			want:     false,
		},
		{
			name:     "empty expected",
			expected: []string{},
			received: []string{"cmd"},
			want:     false,
		},
		{
			name:     "empty received",
			expected: []string{"cmd"},
			received: []string{},
			want:     false,
		},
		{
			name:     "both empty",
			expected: []string{},
			received: []string{},
			want:     true,
		},
		{
			name:     "nil expected",
			expected: nil,
			received: []string{"cmd"},
			want:     false,
		},
		{
			name:     "nil received",
			expected: []string{"cmd"},
			received: nil,
			want:     false,
		},
		{
			name:     "case sensitive mismatch",
			expected: []string{"Kubectl", "get", "pods"},
			received: []string{"kubectl", "get", "pods"},
			want:     false,
		},
		{
			name:     "whitespace differences",
			expected: []string{"kubectl", "get", "pods"},
			received: []string{"kubectl", "get ", "pods"},
			want:     false,
		},
		{
			name:     "special characters",
			expected: []string{"cmd", "--flag=value", "-n", "my-namespace"},
			received: []string{"cmd", "--flag=value", "-n", "my-namespace"},
			want:     true,
		},
		{
			name:     "quoted arguments preserved",
			expected: []string{"echo", "hello world"},
			received: []string{"echo", "hello world"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ArgvMatch(tt.expected, tt.received)
			assert.Equal(t, tt.want, got)
		})
	}
}

const testArg = "arg"

func TestArgvMatch_EdgeCases(t *testing.T) {
	// Test with very long argument lists
	t.Run("long argument list match", func(t *testing.T) {
		expected := make([]string, 100)
		received := make([]string, 100)
		for i := 0; i < 100; i++ {
			expected[i] = testArg
			received[i] = testArg
		}
		assert.True(t, ArgvMatch(expected, received))
	})

	t.Run("long argument list mismatch at end", func(t *testing.T) {
		expected := make([]string, 100)
		received := make([]string, 100)
		for i := 0; i < 100; i++ {
			expected[i] = testArg
			received[i] = testArg
		}
		received[99] = "different"
		assert.False(t, ArgvMatch(expected, received))
	})
}
