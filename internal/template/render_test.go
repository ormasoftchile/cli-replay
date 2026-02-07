package template

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender_SimpleVariable(t *testing.T) {
	vars := map[string]string{
		"name": "world",
	}

	result, err := Render("Hello, {{ .name }}!", vars)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", result)
}

func TestRender_MultipleVariables(t *testing.T) {
	vars := map[string]string{
		"namespace": "production",
		"cluster":   "prod-eus2",
	}

	result, err := Render("Deploying to {{ .namespace }} in {{ .cluster }}", vars)
	require.NoError(t, err)
	assert.Equal(t, "Deploying to production in prod-eus2", result)
}

func TestRender_NoVariables(t *testing.T) {
	result, err := Render("No variables here", nil)
	require.NoError(t, err)
	assert.Equal(t, "No variables here", result)
}

func TestRender_MissingVariable(t *testing.T) {
	vars := map[string]string{
		"exists": "value",
	}

	_, err := Render("{{ .missing }}", vars)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestRender_EmptyTemplate(t *testing.T) {
	result, err := Render("", nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestRender_MultilineTemplate(t *testing.T) {
	vars := map[string]string{
		"status": "Running",
		"name":   "web-0",
	}

	template := `NAME    STATUS
{{ .name }}    {{ .status }}`

	result, err := Render(template, vars)
	require.NoError(t, err)
	assert.Contains(t, result, "web-0")
	assert.Contains(t, result, "Running")
}

func TestMergeVars_EnvOverridesVars(t *testing.T) {
	vars := map[string]string{
		"cluster":   "dev",
		"namespace": "default",
	}

	// Set environment variable
	require.NoError(t, os.Setenv("cluster", "prod-override"))
	defer func() { _ = os.Unsetenv("cluster") }()

	merged := MergeVars(vars)

	// Environment should override
	assert.Equal(t, "prod-override", merged["cluster"])
	// Non-overridden should remain
	assert.Equal(t, "default", merged["namespace"])
}

func TestMergeVars_NilVars(t *testing.T) {
	merged := MergeVars(nil)
	assert.NotNil(t, merged)
}

func TestMergeVars_EmptyVars(t *testing.T) {
	merged := MergeVars(map[string]string{})
	assert.NotNil(t, merged)
}

func TestRenderWithEnv(t *testing.T) {
	vars := map[string]string{
		"base_var": "from_yaml",
	}

	// Set env override
	require.NoError(t, os.Setenv("base_var", "from_env"))
	defer func() { _ = os.Unsetenv("base_var") }()

	result, err := RenderWithEnv("Value: {{ .base_var }}", vars)
	require.NoError(t, err)
	assert.Equal(t, "Value: from_env", result)
}

func TestRender_SpecialCharacters(t *testing.T) {
	vars := map[string]string{
		"path": "/usr/local/bin",
	}

	result, err := Render("PATH={{ .path }}", vars)
	require.NoError(t, err)
	assert.Equal(t, "PATH=/usr/local/bin", result)
}

func TestRender_QuotedValues(t *testing.T) {
	vars := map[string]string{
		"message": "Hello \"world\"",
	}

	result, err := Render("{{ .message }}", vars)
	require.NoError(t, err)
	assert.Equal(t, "Hello \"world\"", result)
}

// T008/T011: MergeVarsFiltered tests

func TestMergeVarsFiltered_NoPatternsAsExistingBehavior(t *testing.T) {
	vars := map[string]string{"cluster": "dev", "namespace": "default"}

	require.NoError(t, os.Setenv("cluster", "prod-override"))
	defer func() { _ = os.Unsetenv("cluster") }()

	merged, denied := MergeVarsFiltered(vars, nil)
	assert.Equal(t, "prod-override", merged["cluster"])
	assert.Equal(t, "default", merged["namespace"])
	assert.Empty(t, denied)
}

func TestMergeVarsFiltered_EmptyPatternsAsExistingBehavior(t *testing.T) {
	vars := map[string]string{"cluster": "dev"}

	require.NoError(t, os.Setenv("cluster", "prod"))
	defer func() { _ = os.Unsetenv("cluster") }()

	merged, denied := MergeVarsFiltered(vars, []string{})
	assert.Equal(t, "prod", merged["cluster"])
	assert.Empty(t, denied)
}

func TestMergeVarsFiltered_DeniedVarKeepsOriginal(t *testing.T) {
	vars := map[string]string{"AWS_KEY": "yaml-default", "HOME": "yaml-home"}

	require.NoError(t, os.Setenv("AWS_KEY", "real-secret"))
	require.NoError(t, os.Setenv("HOME", "/real/home"))
	defer func() {
		_ = os.Unsetenv("AWS_KEY")
		_ = os.Unsetenv("HOME")
	}()

	merged, denied := MergeVarsFiltered(vars, []string{"AWS_*"})

	// AWS_KEY denied → keeps yaml-default
	assert.Equal(t, "yaml-default", merged["AWS_KEY"])
	// HOME not denied → gets env override
	assert.Equal(t, "/real/home", merged["HOME"])
	// Denied list should contain AWS_KEY
	assert.Contains(t, denied, "AWS_KEY")
	assert.Len(t, denied, 1)
}

func TestMergeVarsFiltered_DenyAllWildcard(t *testing.T) {
	vars := map[string]string{
		"SECRET":    "base-secret",
		"NAMESPACE": "base-ns",
	}

	require.NoError(t, os.Setenv("SECRET", "real-secret"))
	require.NoError(t, os.Setenv("NAMESPACE", "real-ns"))
	defer func() {
		_ = os.Unsetenv("SECRET")
		_ = os.Unsetenv("NAMESPACE")
	}()

	merged, denied := MergeVarsFiltered(vars, []string{"*"})

	// All denied → keeps base values
	assert.Equal(t, "base-secret", merged["SECRET"])
	assert.Equal(t, "base-ns", merged["NAMESPACE"])
	assert.Len(t, denied, 2)
}

func TestMergeVarsFiltered_ExemptVarsNotDenied(t *testing.T) {
	vars := map[string]string{
		"CLI_REPLAY_SESSION": "base-session",
		"SOME_VAR":           "base-val",
	}

	require.NoError(t, os.Setenv("CLI_REPLAY_SESSION", "real-session"))
	require.NoError(t, os.Setenv("SOME_VAR", "real-val"))
	defer func() {
		_ = os.Unsetenv("CLI_REPLAY_SESSION")
		_ = os.Unsetenv("SOME_VAR")
	}()

	merged, denied := MergeVarsFiltered(vars, []string{"*"})

	// CLI_REPLAY_SESSION is exempt — gets env override even with deny-all
	assert.Equal(t, "real-session", merged["CLI_REPLAY_SESSION"])
	// SOME_VAR is denied
	assert.Equal(t, "base-val", merged["SOME_VAR"])
	assert.Contains(t, denied, "SOME_VAR")
	assert.NotContains(t, denied, "CLI_REPLAY_SESSION")
}

func TestMergeVarsFiltered_NoEnvVarPresent(t *testing.T) {
	// Env vars with unique names that won't exist
	vars := map[string]string{"XYZZY_UNIQUE_VAR_123": "base-val"}

	merged, denied := MergeVarsFiltered(vars, []string{"*"})

	// No env var present → base value preserved, not "denied" since nothing was suppressed
	assert.Equal(t, "base-val", merged["XYZZY_UNIQUE_VAR_123"])
	assert.Empty(t, denied)
}

func TestMergeVarsFiltered_MultiplePatterns(t *testing.T) {
	vars := map[string]string{
		"AWS_KEY":      "aws-default",
		"GITHUB_TOKEN": "gh-default",
		"DB_SECRET":    "db-default",
		"NORMAL_VAR":   "normal-default",
	}

	require.NoError(t, os.Setenv("AWS_KEY", "aws-real"))
	require.NoError(t, os.Setenv("GITHUB_TOKEN", "gh-real"))
	require.NoError(t, os.Setenv("DB_SECRET", "db-real"))
	require.NoError(t, os.Setenv("NORMAL_VAR", "normal-real"))
	defer func() {
		_ = os.Unsetenv("AWS_KEY")
		_ = os.Unsetenv("GITHUB_TOKEN")
		_ = os.Unsetenv("DB_SECRET")
		_ = os.Unsetenv("NORMAL_VAR")
	}()

	merged, denied := MergeVarsFiltered(vars, []string{"AWS_*", "GITHUB_TOKEN", "*_SECRET"})

	assert.Equal(t, "aws-default", merged["AWS_KEY"])
	assert.Equal(t, "gh-default", merged["GITHUB_TOKEN"])
	assert.Equal(t, "db-default", merged["DB_SECRET"])
	assert.Equal(t, "normal-real", merged["NORMAL_VAR"])
	assert.Len(t, denied, 3)
}

func TestMergeVarsFiltered_NilVars(t *testing.T) {
	merged, denied := MergeVarsFiltered(nil, []string{"*"})
	assert.NotNil(t, merged)
	assert.Empty(t, merged)
	assert.Empty(t, denied)
}
