// Package matcher provides functions for matching CLI arguments against scenarios.
package matcher

import (
	"fmt"
	"regexp"
	"strings"
)

// regexTemplateRe matches {{ .regex "<pattern>" }} in an argv element.
var regexTemplateRe = regexp.MustCompile(`^\{\{\s*\.regex\s+"(.+?)"\s*\}\}$`)

// ArgvMatch performs comparison of two argument vectors.
// Returns true if both slices have the same length and all elements match.
//
// Matching rules per element:
//   - Literal string: exact equality (default)
//   - {{ .any }}: matches any single argument
//   - {{ .regex "<pattern>" }}: matches if value satisfies the regex
func ArgvMatch(expected, received []string) bool {
	if len(expected) != len(received) {
		return false
	}
	for i := range expected {
		if !elementMatch(expected[i], received[i]) {
			return false
		}
	}
	return true
}

// elementMatch checks if a single expected pattern matches a received value.
func elementMatch(pattern, value string) bool {
	// Fast path: exact literal match (vast majority of cases)
	if pattern == value {
		return true
	}

	// Only check templates if pattern contains "{{"
	if !strings.Contains(pattern, "{{") {
		return false
	}

	// Wildcard: {{ .any }}
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "{{ .any }}" || trimmed == "{{.any}}" {
		return true
	}

	// Regex: {{ .regex "<pattern>" }}
	if m := regexTemplateRe.FindStringSubmatch(trimmed); m != nil {
		re, err := regexp.Compile(m[1])
		if err != nil {
			return false // invalid regex → no match
		}
		return re.MatchString(value)
	}

	return false
}

// MatchDetail contains detailed information about an element match result.
// Used for diagnostics — called only when a mismatch is already detected.
type MatchDetail struct {
	Matched    bool   // Whether the element matched
	Kind       string // "literal", "wildcard", or "regex"
	Pattern    string // The regex pattern string, "{{ .any }}", or empty for literal
	FailReason string // Human-readable explanation of why match failed
}

// ElementMatchDetail returns detailed match information for a single element.
// Unlike elementMatch (hot path, returns bool), this is called only on mismatch
// for the divergence position to generate diagnostic output.
func ElementMatchDetail(pattern, value string) MatchDetail {
	// Check for wildcard
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "{{ .any }}" || trimmed == "{{.any}}" {
		return MatchDetail{
			Matched: true,
			Kind:    "wildcard",
			Pattern: "{{ .any }}",
		}
	}

	// Check for regex template
	if m := regexTemplateRe.FindStringSubmatch(trimmed); m != nil {
		regexPattern := m[1]
		re, err := regexp.Compile(regexPattern)
		if err != nil {
			return MatchDetail{
				Matched:    false,
				Kind:       "regex",
				Pattern:    regexPattern,
				FailReason: fmt.Sprintf("invalid regex %q: %v", regexPattern, err),
			}
		}
		if re.MatchString(value) {
			return MatchDetail{
				Matched: true,
				Kind:    "regex",
				Pattern: regexPattern,
			}
		}
		return MatchDetail{
			Matched:    false,
			Kind:       "regex",
			Pattern:    regexPattern,
			FailReason: fmt.Sprintf("regex %q did not match %q", regexPattern, value),
		}
	}

	// Literal comparison
	if pattern == value {
		return MatchDetail{
			Matched: true,
			Kind:    "literal",
		}
	}
	return MatchDetail{
		Matched:    false,
		Kind:       "literal",
		FailReason: fmt.Sprintf("expected %q, got %q", pattern, value),
	}
}
