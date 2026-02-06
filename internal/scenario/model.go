// Package scenario provides types and functions for loading and validating
// cli-replay scenario files.
package scenario

import (
	"errors"
	"fmt"
	"strings"
)

// Scenario represents a complete test definition loaded from a YAML file.
type Scenario struct {
	Meta  Meta   `yaml:"meta"`
	Steps []Step `yaml:"steps"`
}

// Validate checks that the scenario is valid.
func (s *Scenario) Validate() error {
	if err := s.Meta.Validate(); err != nil {
		return fmt.Errorf("meta: %w", err)
	}
	if len(s.Steps) == 0 {
		return errors.New("steps must contain at least one step")
	}
	for i, step := range s.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
	}
	return nil
}

// Meta contains scenario metadata including identification and template variables.
type Meta struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Vars        map[string]string `yaml:"vars,omitempty"`
}

// Validate checks that the meta section is valid.
func (m *Meta) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name must be non-empty")
	}
	return nil
}

// Step represents a single command-response pair within a scenario.
type Step struct {
	Match   Match    `yaml:"match"`
	Respond Response `yaml:"respond"`
}

// Validate checks that the step is valid.
func (s *Step) Validate() error {
	if err := s.Match.Validate(); err != nil {
		return fmt.Errorf("match: %w", err)
	}
	if err := s.Respond.Validate(); err != nil {
		return fmt.Errorf("respond: %w", err)
	}
	return nil
}

// Match contains criteria for identifying an incoming CLI command.
type Match struct {
	Argv []string `yaml:"argv"`
}

// Validate checks that the match criteria is valid.
func (m *Match) Validate() error {
	if len(m.Argv) == 0 {
		return errors.New("argv must be non-empty")
	}
	return nil
}

// Response defines the output for a matched command.
type Response struct {
	Exit       int    `yaml:"exit"`
	Stdout     string `yaml:"stdout,omitempty"`
	Stderr     string `yaml:"stderr,omitempty"`
	StdoutFile string `yaml:"stdout_file,omitempty"`
	StderrFile string `yaml:"stderr_file,omitempty"`
}

// Validate checks that the response is valid.
func (r *Response) Validate() error {
	if r.Exit < 0 || r.Exit > 255 {
		return errors.New("exit must be in range 0-255")
	}
	if r.Stdout != "" && r.StdoutFile != "" {
		return errors.New("stdout and stdout_file are mutually exclusive")
	}
	if r.Stderr != "" && r.StderrFile != "" {
		return errors.New("stderr and stderr_file are mutually exclusive")
	}
	return nil
}
