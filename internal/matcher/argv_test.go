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

func TestArgvMatch_Wildcard(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		received []string
		want     bool
	}{
		{
			name:     "any matches any value",
			expected: []string{"az", "group", "list", "--subscription", "{{ .any }}"},
			received: []string{"az", "group", "list", "--subscription", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
			want:     true,
		},
		{
			name:     "any matches empty string",
			expected: []string{"cmd", "{{ .any }}"},
			received: []string{"cmd", ""},
			want:     true,
		},
		{
			name:     "any without spaces",
			expected: []string{"cmd", "{{.any}}"},
			received: []string{"cmd", "anything"},
			want:     true,
		},
		{
			name:     "any does not change length check",
			expected: []string{"cmd", "{{ .any }}"},
			received: []string{"cmd"},
			want:     false,
		},
		{
			name:     "any in first position",
			expected: []string{"{{ .any }}", "get", "pods"},
			received: []string{"kubectl", "get", "pods"},
			want:     true,
		},
		{
			name:     "multiple any wildcards",
			expected: []string{"cmd", "{{ .any }}", "{{ .any }}"},
			received: []string{"cmd", "foo", "bar"},
			want:     true,
		},
		{
			name:     "literal and any mixed",
			expected: []string{"az", "{{ .any }}", "list", "{{ .any }}"},
			received: []string{"az", "group", "list", "--all"},
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

func TestArgvMatch_Regex(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		received []string
		want     bool
	}{
		{
			name:     "uuid regex matches uuid",
			expected: []string{"az", "--sub", `{{ .regex "^[0-9a-f-]{36}$" }}`},
			received: []string{"az", "--sub", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
			want:     true,
		},
		{
			name:     "uuid regex rejects non-uuid",
			expected: []string{"az", "--sub", `{{ .regex "^[0-9a-f-]{36}$" }}`},
			received: []string{"az", "--sub", "not-a-uuid"},
			want:     false,
		},
		{
			name:     "number regex",
			expected: []string{"cmd", `{{ .regex "^[0-9]+$" }}`},
			received: []string{"cmd", "42"},
			want:     true,
		},
		{
			name:     "number regex rejects text",
			expected: []string{"cmd", `{{ .regex "^[0-9]+$" }}`},
			received: []string{"cmd", "abc"},
			want:     false,
		},
		{
			name:     "prefix regex",
			expected: []string{"cmd", `{{ .regex "^my-rg-" }}`},
			received: []string{"cmd", "my-rg-eastus"},
			want:     true,
		},
		{
			name:     "invalid regex is no match",
			expected: []string{"cmd", `{{ .regex "[invalid" }}`},
			received: []string{"cmd", "anything"},
			want:     false,
		},
		{
			name:     "mix of literal any and regex",
			expected: []string{"az", "group", "list", "{{ .any }}", `{{ .regex "^(table|json)$" }}`},
			received: []string{"az", "group", "list", "--output", "table"},
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

func TestElementMatchDetail_LiteralMatch(t *testing.T) {
	d := ElementMatchDetail("pods", "pods")
	assert.True(t, d.Matched)
	assert.Equal(t, "literal", d.Kind)
	assert.Empty(t, d.Pattern)
	assert.Empty(t, d.FailReason)
}

func TestElementMatchDetail_LiteralFail(t *testing.T) {
	d := ElementMatchDetail("pods", "services")
	assert.False(t, d.Matched)
	assert.Equal(t, "literal", d.Kind)
	assert.Empty(t, d.Pattern)
	assert.Contains(t, d.FailReason, `expected "pods"`)
	assert.Contains(t, d.FailReason, `got "services"`)
}

func TestElementMatchDetail_WildcardMatch(t *testing.T) {
	d := ElementMatchDetail("{{ .any }}", "anything-goes")
	assert.True(t, d.Matched)
	assert.Equal(t, "wildcard", d.Kind)
	assert.Equal(t, "{{ .any }}", d.Pattern)
	assert.Empty(t, d.FailReason)
}

func TestElementMatchDetail_WildcardMatchCompact(t *testing.T) {
	d := ElementMatchDetail("{{.any}}", "value")
	assert.True(t, d.Matched)
	assert.Equal(t, "wildcard", d.Kind)
}

func TestElementMatchDetail_RegexMatch(t *testing.T) {
	d := ElementMatchDetail(`{{ .regex "^prod-.*" }}`, "prod-east")
	assert.True(t, d.Matched)
	assert.Equal(t, "regex", d.Kind)
	assert.Equal(t, "^prod-.*", d.Pattern)
	assert.Empty(t, d.FailReason)
}

func TestElementMatchDetail_RegexFail(t *testing.T) {
	d := ElementMatchDetail(`{{ .regex "^prod-.*" }}`, "staging-app")
	assert.False(t, d.Matched)
	assert.Equal(t, "regex", d.Kind)
	assert.Equal(t, "^prod-.*", d.Pattern)
	assert.Contains(t, d.FailReason, "did not match")
	assert.Contains(t, d.FailReason, "staging-app")
}

func TestElementMatchDetail_RegexInvalid(t *testing.T) {
	d := ElementMatchDetail(`{{ .regex "[invalid" }}`, "anything")
	assert.False(t, d.Matched)
	assert.Equal(t, "regex", d.Kind)
	assert.Contains(t, d.FailReason, "invalid regex")
}
