package scenario

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Load parses a scenario from the given reader with strict field validation.
// Unknown fields in the YAML will cause an error.
func Load(r io.Reader) (*Scenario, error) {
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)

	var scenario Scenario
	if err := decoder.Decode(&scenario); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("empty scenario file")
		}
		return nil, fmt.Errorf("failed to parse scenario: %w", err)
	}

	if err := scenario.Validate(); err != nil {
		return nil, fmt.Errorf("invalid scenario: %w", err)
	}

	return &scenario, nil
}

// LoadFile loads a scenario from the given file path.
func LoadFile(path string) (*Scenario, error) {
	f, err := os.Open(path) //nolint:gosec // File path comes from user input, expected behavior
	if err != nil {
		return nil, fmt.Errorf("failed to open scenario file: %w", err)
	}
	defer func() { _ = f.Close() }()

	scenario, err := Load(f)
	if err != nil {
		return nil, err
	}

	return scenario, nil
}
