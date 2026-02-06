// Package template provides Go text/template rendering for cli-replay responses.
package template

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
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
