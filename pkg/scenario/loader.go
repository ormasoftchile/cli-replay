package scenario

import (
	"bytes"
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

// MarshalYAML implements custom YAML marshaling for StepElement.
// It serializes the underlying Step or group wrapper so that fields
// tagged yaml:"-" are emitted correctly.
func (se StepElement) MarshalYAML() (interface{}, error) {
	if se.Group != nil {
		return struct {
			Group *StepGroup `yaml:"group"`
		}{Group: se.Group}, nil
	}
	if se.Step != nil {
		return se.Step, nil
	}
	return nil, fmt.Errorf("step element has neither step nor group")
}

// UnmarshalYAML implements custom YAML unmarshaling for StepElement.
// It inspects the mapping keys to determine whether the node represents
// a leaf step (has "match") or a group (has "group"), then dispatches
// to the appropriate type.
func (se *StepElement) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("step element must be a mapping, got %v", value.Kind)
	}

	// Scan mapping keys to detect "group" key
	isGroup := false
	for i := 0; i < len(value.Content)-1; i += 2 {
		if value.Content[i].Value == "group" {
			isGroup = true
			break
		}
	}

	if isGroup {
		// Validate known fields for group wrapper
		for i := 0; i < len(value.Content)-1; i += 2 {
			key := value.Content[i].Value
			if key != "group" {
				return fmt.Errorf("line %d: field %s not found in type step (group wrapper)", value.Content[i].Line, key)
			}
		}
		// Decode as group wrapper: { group: { mode, name, steps } }
		var wrapper struct {
			Group StepGroup `yaml:"group"`
		}
		if err := value.Decode(&wrapper); err != nil {
			return fmt.Errorf("failed to decode group: %w", err)
		}
		se.Group = &wrapper.Group
		return nil
	}

	// Decode as leaf step with strict field checking.
	// Re-encode the node to bytes, then decode with KnownFields(true)
	// so that unknown fields in step, match, and respond are rejected.
	step, err := strictDecodeStep(value)
	if err != nil {
		return err
	}
	se.Step = step
	return nil
}

// UnmarshalYAML implements custom YAML unmarshaling for StepGroup.
// It decodes the group's mode, name, and steps fields.
func (sg *StepGroup) UnmarshalYAML(value *yaml.Node) error {
	// Use an intermediate type to avoid infinite recursion
	type rawStepGroup StepGroup
	var raw rawStepGroup
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*sg = StepGroup(raw)
	return nil
}

// strictDecodeStep re-encodes a yaml.Node to bytes and decodes it with
// KnownFields(true) so that unknown fields at any nesting level (step,
// match, respond) are rejected — preserving the strict-parsing behavior
// that yaml.Decoder.KnownFields(true) provides at the top level.
func strictDecodeStep(node *yaml.Node) (*Step, error) {
	data, err := yaml.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("failed to encode step node: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var step Step
	if err := dec.Decode(&step); err != nil {
		return nil, fmt.Errorf("failed to decode step: %w", err)
	}
	return &step, nil
}
