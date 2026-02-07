// Package matcher provides functions for matching CLI arguments against scenarios.
package matcher

import (
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
