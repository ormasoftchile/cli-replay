// Package envfilter provides glob-based environment variable filtering
// for cli-replay. It allows deny-listing env var names using path.Match
// glob patterns while exempting cli-replay's own internal variables.
package envfilter

import (
	"path"
	"strings"
)

// internalPrefixes lists env var prefixes that are always exempt from
// deny filtering. These are cli-replay's own control variables.
var internalPrefixes = []string{
	"CLI_REPLAY_SESSION",
	"CLI_REPLAY_SCENARIO",
	"CLI_REPLAY_RECORDING_LOG",
	"CLI_REPLAY_SHIM_DIR",
	"CLI_REPLAY_TRACE",
}

// IsDenied returns true if the environment variable name matches any of the
// provided deny-list glob patterns. Uses path.Match for glob matching, which
// handles * wildcards correctly for non-path strings (env var names don't
// contain /). Returns false for invalid patterns (fail-open).
//
// An exempt variable (see IsExempt) is never denied regardless of patterns.
func IsDenied(name string, patterns []string) bool {
	if IsExempt(name) {
		return false
	}
	for _, pattern := range patterns {
		matched, err := path.Match(pattern, name)
		if err != nil {
			// Invalid pattern — skip (fail-open)
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// IsExempt returns true if the environment variable name is a cli-replay
// internal variable that must never be filtered. This ensures that deny
// patterns like "*" don't break cli-replay's own operation.
func IsExempt(name string) bool {
	for _, prefix := range internalPrefixes {
		if strings.EqualFold(name, prefix) {
			return true
		}
	}
	return false
}
