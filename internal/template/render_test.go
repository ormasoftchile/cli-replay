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
