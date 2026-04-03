package matcher

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Public API surface tests for pkg/matcher — consumer-perspective tests
// documenting the contract that gert (and other consumers) will depend on.
// ═══════════════════════════════════════════════════════════════════════════════

// --- ArgvMatch: mixed pattern types -----------------------------------------

//nolint:funlen // Table-driven test with comprehensive cases
func TestArgvMatch_MixedPatterns_Table(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		received []string
		want     bool
	}{
		{
			name:     "literal + wildcard + regex all match",
			expected: []string{"az", "{{ .any }}", `{{ .regex "^(list|show)$" }}`},
			received: []string{"az", "group", "list"},
			want:     true,
		},
		{
			name:     "literal + wildcard + regex: regex fails",
			expected: []string{"az", "{{ .any }}", `{{ .regex "^(list|show)$" }}`},
			received: []string{"az", "group", "delete"},
			want:     false,
		},
		{
			name:     "all wildcards",
			expected: []string{"{{ .any }}", "{{ .any }}", "{{ .any }}"},
			received: []string{"any", "thing", "goes"},
			want:     true,
		},
		{
			name:     "all regex",
			expected: []string{`{{ .regex "^k" }}`, `{{ .regex "^g" }}`, `{{ .regex "^p" }}`},
			received: []string{"kubectl", "get", "pods"},
			want:     true,
		},
		{
			name:     "flag value with regex for path-like args",
			expected: []string{"kubectl", "apply", "-f", `{{ .regex "^/tmp/" }}`},
			received: []string{"kubectl", "apply", "-f", "/tmp/deploy.yaml"},
			want:     true,
		},
		{
			name:     "empty argv match",
			expected: []string{},
			received: []string{},
			want:     true,
		},
		{
			name:     "single element wildcard",
			expected: []string{"{{ .any }}"},
			received: []string{"anything"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ArgvMatch(tt.expected, tt.received))
		})
	}
}

// --- ArgvMatch: concurrent safety -------------------------------------------

func TestArgvMatch_ConcurrentSafety(t *testing.T) {
	expected := []string{"kubectl", "get", "{{ .any }}", `{{ .regex "^ns-" }}`}
	received := []string{"kubectl", "get", "pods", "ns-default"}

	var wg sync.WaitGroup
	const goroutines = 100
	results := make([]bool, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = ArgvMatch(expected, received)
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		assert.True(t, r, "goroutine %d should have matched", i)
	}
}

// --- ElementMatchDetail: comprehensive cases --------------------------------

//nolint:funlen // Table-driven test
func TestElementMatchDetail_Comprehensive(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		value      string
		wantMatch  bool
		wantKind   string
		wantReason string
	}{
		{
			name:      "literal exact match",
			pattern:   "pods",
			value:     "pods",
			wantMatch: true,
			wantKind:  "literal",
		},
		{
			name:       "literal mismatch",
			pattern:    "pods",
			value:      "services",
			wantMatch:  false,
			wantKind:   "literal",
			wantReason: `expected "pods"`,
		},
		{
			name:      "wildcard spaced",
			pattern:   "{{ .any }}",
			value:     "anything",
			wantMatch: true,
			wantKind:  "wildcard",
		},
		{
			name:      "wildcard compact",
			pattern:   "{{.any}}",
			value:     "",
			wantMatch: true,
			wantKind:  "wildcard",
		},
		{
			name:      "regex match",
			pattern:   `{{ .regex "^[0-9]+$" }}`,
			value:     "42",
			wantMatch: true,
			wantKind:  "regex",
		},
		{
			name:       "regex no match",
			pattern:    `{{ .regex "^[0-9]+$" }}`,
			value:      "abc",
			wantMatch:  false,
			wantKind:   "regex",
			wantReason: "did not match",
		},
		{
			name:       "regex invalid pattern",
			pattern:    `{{ .regex "[invalid" }}`,
			value:      "anything",
			wantMatch:  false,
			wantKind:   "regex",
			wantReason: "invalid regex",
		},
		{
			name:      "regex with escaped dots",
			pattern:   `{{ .regex "^https?://.*\.example\.com" }}`,
			value:     "https://api.example.com",
			wantMatch: true,
			wantKind:  "regex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ElementMatchDetail(tt.pattern, tt.value)
			assert.Equal(t, tt.wantMatch, d.Matched, "Matched")
			assert.Equal(t, tt.wantKind, d.Kind, "Kind")
			if tt.wantReason != "" {
				assert.Contains(t, d.FailReason, tt.wantReason)
			}
		})
	}
}

// --- Edge cases for pattern boundaries --------------------------------------

func TestArgvMatch_PatternBoundary(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		received []string
		want     bool
	}{
		{
			name:     "partial template not treated as wildcard",
			expected: []string{"{{ .any"},
			received: []string{"anything"},
			want:     false,
		},
		{
			name:     "double braces without template function",
			expected: []string{"{{ .unknown }}"},
			received: []string{"anything"},
			want:     false,
		},
		{
			name:     "template with extra whitespace not matched",
			expected: []string{"{{   .any   }}"},
			received: []string{"value"},
			want:     false, // only {{ .any }} and {{.any}} are recognized
		},
		{
			name:     "literal curly braces not confused as template",
			expected: []string{`{value}`},
			received: []string{`{value}`},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ArgvMatch(tt.expected, tt.received))
		})
	}
}
