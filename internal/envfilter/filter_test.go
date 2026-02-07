package envfilter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDenied_PrefixWildcard(t *testing.T) {
	patterns := []string{"AWS_*"}
	assert.True(t, IsDenied("AWS_SECRET_ACCESS_KEY", patterns))
	assert.True(t, IsDenied("AWS_REGION", patterns))
	assert.False(t, IsDenied("GITHUB_TOKEN", patterns))
	assert.False(t, IsDenied("aws_secret", patterns)) // case-sensitive
}

func TestIsDenied_SuffixWildcard(t *testing.T) {
	patterns := []string{"*_TOKEN"}
	assert.True(t, IsDenied("GITHUB_TOKEN", patterns))
	assert.True(t, IsDenied("NPM_TOKEN", patterns))
	assert.False(t, IsDenied("TOKEN_NAME", patterns))
}

func TestIsDenied_ExactMatch(t *testing.T) {
	patterns := []string{"SECRET_KEY"}
	assert.True(t, IsDenied("SECRET_KEY", patterns))
	assert.False(t, IsDenied("SECRET_KEY_2", patterns))
	assert.False(t, IsDenied("MY_SECRET_KEY", patterns))
}

func TestIsDenied_WildcardAll(t *testing.T) {
	patterns := []string{"*"}
	assert.True(t, IsDenied("ANY_VAR", patterns))
	assert.True(t, IsDenied("x", patterns))
	// Internal vars are exempt even with wildcard-all
	assert.False(t, IsDenied("CLI_REPLAY_SESSION", patterns))
	assert.False(t, IsDenied("CLI_REPLAY_TRACE", patterns))
}

func TestIsDenied_MultiplePatterns(t *testing.T) {
	patterns := []string{"AWS_*", "GITHUB_TOKEN", "*_SECRET"}
	assert.True(t, IsDenied("AWS_REGION", patterns))
	assert.True(t, IsDenied("GITHUB_TOKEN", patterns))
	assert.True(t, IsDenied("DB_SECRET", patterns))
	assert.False(t, IsDenied("HOME", patterns))
}

func TestIsDenied_MidWildcard(t *testing.T) {
	patterns := []string{"DB_*_PASSWORD"}
	assert.True(t, IsDenied("DB_PROD_PASSWORD", patterns))
	assert.True(t, IsDenied("DB_DEV_PASSWORD", patterns))
	assert.False(t, IsDenied("DB_PASSWORD", patterns)) // no middle segment
}

func TestIsDenied_EmptyPatterns(t *testing.T) {
	assert.False(t, IsDenied("ANY_VAR", nil))
	assert.False(t, IsDenied("ANY_VAR", []string{}))
}

func TestIsDenied_InvalidPattern(t *testing.T) {
	// path.Match returns error for malformed patterns like `[`
	patterns := []string{"[invalid"}
	assert.False(t, IsDenied("ANY_VAR", patterns)) // fail-open
}

func TestIsDenied_InvalidPatternWithValidPattern(t *testing.T) {
	// Invalid pattern is skipped, valid pattern still matches
	patterns := []string{"[invalid", "AWS_*"}
	assert.True(t, IsDenied("AWS_KEY", patterns))
	assert.False(t, IsDenied("HOME", patterns))
}

func TestIsExempt_InternalVars(t *testing.T) {
	assert.True(t, IsExempt("CLI_REPLAY_SESSION"))
	assert.True(t, IsExempt("CLI_REPLAY_SCENARIO"))
	assert.True(t, IsExempt("CLI_REPLAY_RECORDING_LOG"))
	assert.True(t, IsExempt("CLI_REPLAY_SHIM_DIR"))
	assert.True(t, IsExempt("CLI_REPLAY_TRACE"))
}

func TestIsExempt_CaseInsensitive(t *testing.T) {
	assert.True(t, IsExempt("cli_replay_session"))
	assert.True(t, IsExempt("Cli_Replay_Trace"))
}

func TestIsExempt_NonInternalVars(t *testing.T) {
	assert.False(t, IsExempt("HOME"))
	assert.False(t, IsExempt("AWS_SECRET"))
	assert.False(t, IsExempt("CLI_REPLAY_CUSTOM"))
	assert.False(t, IsExempt(""))
}

func TestIsDenied_ExemptVarsNeverDenied(t *testing.T) {
	// Even with deny-all pattern, internal vars are exempt
	patterns := []string{"*"}
	for _, name := range internalPrefixes {
		assert.False(t, IsDenied(name, patterns), "internal var %s should be exempt", name)
	}
}

func TestIsDenied_EmptyName(t *testing.T) {
	patterns := []string{"*"}
	// Empty string matches * but is not exempt
	assert.True(t, IsDenied("", patterns))
}
