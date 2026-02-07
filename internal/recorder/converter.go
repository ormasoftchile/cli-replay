package recorder

import (
	"fmt"
	"os"
	"strings"

	"github.com/cli-replay/cli-replay/internal/scenario"
	"gopkg.in/yaml.v3"
)

// ConvertToScenario transforms recorded commands into a scenario.Scenario.
func ConvertToScenario(meta SessionMetadata, commands []RecordedCommand) (*scenario.Scenario, error) {
	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("invalid metadata: %w", err)
	}

	// Create scenario with metadata
	sc := &scenario.Scenario{
		Meta: scenario.Meta{
			Name:        meta.Name,
			Description: meta.Description,
		},
		Steps: make([]scenario.StepElement, 0, len(commands)),
	}

	// Convert each recorded command to a scenario step
	for _, cmd := range commands {
		step := scenario.Step{
			Match: scenario.Match{
				Argv:  cmd.Argv,
				Stdin: cmd.Stdin, // populated when non-empty
			},
			Respond: scenario.Response{
				Exit:   cmd.ExitCode,
				Stdout: cmd.Stdout,
				Stderr: cmd.Stderr,
			},
		}

		sc.Steps = append(sc.Steps, scenario.StepElement{Step: &step})
	}

	return sc, nil
}

// GenerateYAML serializes a scenario to YAML format.
func GenerateYAML(sc *scenario.Scenario) (string, error) {
	if sc == nil {
		return "", fmt.Errorf("scenario cannot be nil")
	}

	// Marshal to YAML
	data, err := yaml.Marshal(sc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal scenario to YAML: %w", err)
	}

	return string(data), nil
}

// WriteYAMLFile writes a scenario to a YAML file.
func WriteYAMLFile(outputPath string, sc *scenario.Scenario) error {
	if sc == nil {
		return fmt.Errorf("scenario cannot be nil")
	}

	// Validate scenario before writing
	if err := sc.Validate(); err != nil {
		// Allow empty steps for now (user might record nothing)
		if !strings.Contains(err.Error(), "steps must contain at least one step") {
			return fmt.Errorf("invalid scenario: %w", err)
		}
	}

	// Generate YAML content
	yamlContent, err := GenerateYAML(sc)
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(outputPath, []byte(yamlContent), 0600); err != nil {
		return fmt.Errorf("failed to write YAML file: %w", err)
	}

	return nil
}
