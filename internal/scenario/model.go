// Package scenario provides types and functions for loading and validating
// cli-replay scenario files.
package scenario

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Session defines session lifecycle configuration.
type Session struct {
	TTL string `yaml:"ttl,omitempty"`
}

// Validate checks that the session configuration is valid.
func (s *Session) Validate() error {
	if s.TTL != "" {
		d, err := time.ParseDuration(s.TTL)
		if err != nil {
			return fmt.Errorf("invalid ttl %q: %w", s.TTL, err)
		}
		if d <= 0 {
			return fmt.Errorf("ttl must be positive, got %s", s.TTL)
		}
	}
	return nil
}

// Scenario represents a complete test definition loaded from a YAML file.
type Scenario struct {
	Meta  Meta          `yaml:"meta"`
	Steps []StepElement `yaml:"steps"`
}

// Validate checks that the scenario is valid.
func (s *Scenario) Validate() error {
	if err := s.Meta.Validate(); err != nil {
		return fmt.Errorf("meta: %w", err)
	}
	if len(s.Steps) == 0 {
		return errors.New("steps must contain at least one step")
	}
	groupIdx := 0
	for i, elem := range s.Steps {
		if err := elem.Validate(); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
		// Auto-name groups
		if elem.Group != nil && elem.Group.Name == "" {
			groupIdx++
			s.Steps[i].Group.Name = fmt.Sprintf("group-%d", groupIdx)
		} else if elem.Group != nil {
			groupIdx++
		}
	}
	return nil
}

// FlatSteps returns all leaf steps expanded inline. Groups are replaced by
// their child steps in order. The result preserves contiguous flat indices.
func (s *Scenario) FlatSteps() []Step {
	var flat []Step
	for _, elem := range s.Steps {
		if elem.Step != nil {
			flat = append(flat, *elem.Step)
		} else if elem.Group != nil {
			for _, child := range elem.Group.Steps {
				if child.Step != nil {
					flat = append(flat, *child.Step)
				}
			}
		}
	}
	return flat
}

// GroupRange describes the flat-index extent of a single step group.
type GroupRange struct {
	Start    int    // Inclusive flat index of first group child
	End      int    // Exclusive flat index (Start + len(group.Steps))
	Name     string // Group name (resolved, never empty)
	TopIndex int    // Index of the group in the top-level Steps array
}

// GroupRanges returns the flat-index ranges for all groups in the scenario.
func (s *Scenario) GroupRanges() []GroupRange {
	var ranges []GroupRange
	flatIdx := 0
	for i, elem := range s.Steps {
		if elem.Step != nil {
			flatIdx++
		} else if elem.Group != nil {
			childCount := 0
			for _, child := range elem.Group.Steps {
				if child.Step != nil {
					childCount++
				}
			}
			ranges = append(ranges, GroupRange{
				Start:    flatIdx,
				End:      flatIdx + childCount,
				Name:     elem.Group.Name,
				TopIndex: i,
			})
			flatIdx += childCount
		}
	}
	return ranges
}

// StepElement is a union type — exactly one of Step or Group is non-nil.
// It represents either a leaf step or a group container in the steps array.
type StepElement struct {
	Step  *Step      `yaml:"-"` // Set when YAML has match/respond (leaf step)
	Group *StepGroup `yaml:"-"` // Set when YAML has group key
}

// Validate checks that exactly one of Step or Group is set and validates it.
func (se *StepElement) Validate() error {
	if se.Step == nil && se.Group == nil {
		return errors.New("step element must have either a step or a group")
	}
	if se.Step != nil && se.Group != nil {
		return errors.New("step element must have either a step or a group, not both")
	}
	if se.Step != nil {
		return se.Step.Validate()
	}
	return se.Group.Validate()
}

// StepGroup defines a group of steps with unordered matching semantics.
type StepGroup struct {
	Mode  string        `yaml:"mode"`
	Name  string        `yaml:"name,omitempty"`
	Steps []StepElement `yaml:"steps"`
}

// Validate checks that the step group is valid.
func (sg *StepGroup) Validate() error {
	if sg.Mode != "unordered" {
		return fmt.Errorf("unsupported group mode %q: only \"unordered\" is supported", sg.Mode)
	}
	if len(sg.Steps) == 0 {
		return errors.New("group must contain at least one step")
	}
	for i, elem := range sg.Steps {
		if elem.Group != nil {
			return fmt.Errorf("step %d: nested groups are not allowed", i)
		}
		if elem.Step == nil {
			return fmt.Errorf("step %d: group children must be leaf steps", i)
		}
		if err := elem.Step.Validate(); err != nil {
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
	Session     *Session          `yaml:"session,omitempty"`
}

// Security defines constraints on which commands may be intercepted.
type Security struct {
	AllowedCommands []string `yaml:"allowed_commands,omitempty"`
	DenyEnvVars     []string `yaml:"deny_env_vars,omitempty"`
}

// Validate checks that the meta section is valid.
func (m *Meta) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name must be non-empty")
	}
	if m.Security != nil {
		if err := m.Security.Validate(); err != nil {
			return fmt.Errorf("security: %w", err)
		}
	}
	if m.Session != nil {
		if err := m.Session.Validate(); err != nil {
			return fmt.Errorf("session: %w", err)
		}
	}
	return nil
}

// Validate checks that the security configuration is valid.
func (s *Security) Validate() error {
	for i, pattern := range s.DenyEnvVars {
		if pattern == "" {
			return fmt.Errorf("deny_env_vars[%d]: must be non-empty", i)
		}
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
