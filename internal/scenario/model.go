// Package scenario provides types and functions for loading and validating
// cli-replay scenario files.
package scenario

import (
	"errors"
	"fmt"
	"strings"
	"time"
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
	Security    *Security         `yaml:"security,omitempty"`
}

// Security defines constraints on which commands may be intercepted.
type Security struct {
	AllowedCommands []string `yaml:"allowed_commands,omitempty"`
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
	Match   Match       `yaml:"match"`
	Respond Response    `yaml:"respond"`
	Calls   *CallBounds `yaml:"calls,omitempty"`
	When    string      `yaml:"when,omitempty"`
}

// CallBounds specifies the allowed invocation range for a step.
// When nil on a Step, EffectiveCalls() returns {Min: 1, Max: 1}.
type CallBounds struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

// EffectiveCalls returns the call bounds for this step, applying defaults
// when the Calls field is nil (backward compatible: exactly one call).
func (s *Step) EffectiveCalls() CallBounds {
	if s.Calls == nil {
		return CallBounds{Min: 1, Max: 1}
	}
	return *s.Calls
}

// Validate checks that the call bounds are valid.
func (cb *CallBounds) Validate() error {
	if cb.Min < 0 {
		return fmt.Errorf("min must be >= 0, got %d", cb.Min)
	}
	if cb.Max < 1 {
		return fmt.Errorf("max must be >= 1, got %d", cb.Max)
	}
	if cb.Min > cb.Max {
		return fmt.Errorf("min (%d) must be <= max (%d)", cb.Min, cb.Max)
	}
	return nil
}

// Validate checks that the step is valid.
func (s *Step) Validate() error {
	if err := s.Match.Validate(); err != nil {
		return fmt.Errorf("match: %w", err)
	}
	if err := s.Respond.Validate(); err != nil {
		return fmt.Errorf("respond: %w", err)
	}
	if s.Calls != nil {
		// Apply defaulting: if only min is given (max == 0), default max to min
		if s.Calls.Max == 0 && s.Calls.Min > 0 {
			s.Calls.Max = s.Calls.Min
		}
		if err := s.Calls.Validate(); err != nil {
			return fmt.Errorf("calls: %w", err)
		}
	}
	return nil
}

// Match contains criteria for identifying an incoming CLI command.
type Match struct {
	Argv  []string `yaml:"argv"`
	Stdin string   `yaml:"stdin,omitempty"`
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
	Delay      string `yaml:"delay,omitempty"`
}

// ValidateDelay checks that the delay does not exceed the given maximum.
// A zero maxDelay disables the cap. Returns nil if no delay is set.
func (r *Response) ValidateDelay(maxDelay time.Duration) error {
	if r.Delay == "" || maxDelay == 0 {
		return nil
	}
	d, err := time.ParseDuration(r.Delay)
	if err != nil {
		return fmt.Errorf("invalid delay %q: %w", r.Delay, err)
	}
	if d > maxDelay {
		return fmt.Errorf("delay %s exceeds max-delay %s", r.Delay, maxDelay)
	}
	return nil
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
