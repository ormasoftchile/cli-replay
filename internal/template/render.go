// Package template provides Go text/template rendering for cli-replay responses.
package template

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"github.com/cli-replay/cli-replay/internal/envfilter"
)

// Render renders a Go text/template with the given variables.
// Uses missingkey=error to fail on undefined variables.
func Render(tmpl string, vars map[string]string) (string, error) {
	if tmpl == "" {
		return "", nil
	}

	t, err := template.New("response").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Convert map[string]string to map[string]interface{} for template execution
	data := make(map[string]interface{}, len(vars))
	for k, v := range vars {
		data[k] = v
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// RenderWithEnv renders a template with variables merged from vars and environment.
// Environment variables override vars.
func RenderWithEnv(tmpl string, vars map[string]string) (string, error) {
	merged := MergeVars(vars)
	return Render(tmpl, merged)
}

// MergeVars merges scenario vars with environment variables.
// Environment variables override scenario vars.
func MergeVars(vars map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy base vars
	for k, v := range vars {
		result[k] = v
	}

	// Override with environment variables
	for k := range result {
		if envVal := os.Getenv(k); envVal != "" {
			result[k] = envVal
		}
	}

	return result
}

// MergeVarsFiltered merges scenario vars with environment variables, but
// suppresses env var overrides for names that match any of the deny patterns.
// When an env var is denied, the original meta.vars value is preserved
// (or empty string if no base value exists).
// Returns the merged map and a slice of denied variable names for trace output.
// If denyPatterns is nil/empty, behaves identically to MergeVars.
func MergeVarsFiltered(vars map[string]string, denyPatterns []string) (map[string]string, []string) {
	result := make(map[string]string)
	var denied []string

	// Copy base vars
	for k, v := range vars {
		result[k] = v
	}

	// Override with environment variables, respecting deny patterns
	for k := range result {
		if envVal := os.Getenv(k); envVal != "" {
			if len(denyPatterns) > 0 && envfilter.IsDenied(k, denyPatterns) {
				// Denied: keep the meta.vars value (or empty if none)
				denied = append(denied, k)
			} else {
				result[k] = envVal
			}
		}
	}

	return result, denied
}
