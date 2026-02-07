package runner

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChildEnv_PrependsPATH(t *testing.T) {
	env := BuildChildEnv("/intercept/dir", "session-abc", "/path/to/scenario.yaml")

	pathSep := ":"
	if runtime.GOOS == "windows" {
		pathSep = ";"
	}

	var pathVal string
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			pathVal = strings.TrimPrefix(e, "PATH=")
			break
		}
	}
	require.NotEmpty(t, pathVal, "PATH should be present in env")
	assert.True(t, strings.HasPrefix(pathVal, "/intercept/dir"+pathSep),
		"PATH should be prepended with intercept dir, got: %s", pathVal)
}

func TestBuildChildEnv_SetsSessionAndScenario(t *testing.T) {
	env := BuildChildEnv("/intercept/dir", "session-xyz", "/scenario.yaml")

	envMap := envToMap(env)
	assert.Equal(t, "session-xyz", envMap["CLI_REPLAY_SESSION"])
	assert.Equal(t, "/scenario.yaml", envMap["CLI_REPLAY_SCENARIO"])
}

func TestBuildChildEnv_PreservesExistingVars(t *testing.T) {
	// Set a known env var to check preservation
	t.Setenv("CLI_REPLAY_TEST_PRESERVE", "keep-me")

	env := BuildChildEnv("/intercept", "s1", "/scn.yaml")
	envMap := envToMap(env)
	assert.Equal(t, "keep-me", envMap["CLI_REPLAY_TEST_PRESERVE"])
}

func TestBuildChildEnv_OverwritesExistingSessionAndScenario(t *testing.T) {
	t.Setenv("CLI_REPLAY_SESSION", "old-session")
	t.Setenv("CLI_REPLAY_SCENARIO", "/old/scenario.yaml")

	env := BuildChildEnv("/intercept", "new-session", "/new/scenario.yaml")
	envMap := envToMap(env)
	assert.Equal(t, "new-session", envMap["CLI_REPLAY_SESSION"])
	assert.Equal(t, "/new/scenario.yaml", envMap["CLI_REPLAY_SCENARIO"])
}

func TestBuildChildEnv_NoDuplicateKeys(t *testing.T) {
	t.Setenv("CLI_REPLAY_SESSION", "old")

	env := BuildChildEnv("/intercept", "new", "/scn.yaml")

	sessionCount := 0
	for _, e := range env {
		if strings.HasPrefix(e, "CLI_REPLAY_SESSION=") {
			sessionCount++
		}
	}
	assert.Equal(t, 1, sessionCount, "CLI_REPLAY_SESSION should appear exactly once")
}

func TestBuildChildEnv_MissingPATH(t *testing.T) {
	// Save and clear PATH, then restore
	origPath := os.Getenv("PATH")
	os.Unsetenv("PATH") //nolint:errcheck
	defer os.Setenv("PATH", origPath) //nolint:errcheck

	env := BuildChildEnv("/intercept", "s1", "/scn.yaml")
	envMap := envToMap(env)

	// PATH should still be set (just the intercept dir)
	assert.Equal(t, "/intercept", envMap["PATH"])
}

func TestSplitEnvVar(t *testing.T) {
	key, val, ok := splitEnvVar("FOO=bar")
	assert.True(t, ok)
	assert.Equal(t, "FOO", key)
	assert.Equal(t, "bar", val)

	key, val, ok = splitEnvVar("FOO=bar=baz")
	assert.True(t, ok)
	assert.Equal(t, "FOO", key)
	assert.Equal(t, "bar=baz", val)

	_, _, ok = splitEnvVar("NOEQUALSSIGN")
	assert.False(t, ok)

	key, val, ok = splitEnvVar("EMPTY=")
	assert.True(t, ok)
	assert.Equal(t, "EMPTY", key)
	assert.Equal(t, "", val)
}

// envToMap converts a []string env slice into a map for easier assertions.
func envToMap(env []string) map[string]string {
	m := make(map[string]string)
	for _, e := range env {
		k, v, ok := splitEnvVar(e)
		if ok {
			m[k] = v
		}
	}
	return m
}
