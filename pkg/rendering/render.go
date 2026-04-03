// Package rendering provides Go text/template rendering for cli-replay responses.
// This is the canonical location for template rendering shared by both the
// public replay engine (pkg/replay) and the internal runner (internal/runner).
package rendering

import (
	"bytes"
	"fmt"
	"text/template"
)

// RenderWithCaptures renders a Go text/template with vars and captures.
// Vars are top-level keys, captures are nested under the "capture" namespace.
// Uses missingkey=zero so that unresolved capture references (from optional
// steps or unordered group siblings) resolve to empty string instead of
// erroring.
func RenderWithCaptures(tmpl string, vars map[string]string, captures map[string]string) (string, error) {
	if tmpl == "" {
		return "", nil
	}

	t, err := template.New("response").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	data := make(map[string]interface{}, len(vars)+1)
	for k, v := range vars {
		data[k] = v
	}

	captureMap := make(map[string]string)
	for k, v := range captures {
		captureMap[k] = v
	}
	data["capture"] = captureMap

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.String(), nil
}
