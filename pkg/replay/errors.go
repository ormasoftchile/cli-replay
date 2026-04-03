package replay

import "fmt"

// MismatchError is returned when a command does not match the expected step.
type MismatchError struct {
	StepIndex     int
	Expected      []string
	Received      []string
	SoftAdvanced  bool
	NextStepIndex int
	NextExpected  []string
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("argv mismatch at step %d", e.StepIndex)
}

// GroupMismatchError is returned when a command does not match any step
// within an unordered group and the group's minimum counts are not yet met.
type GroupMismatchError struct {
	GroupName     string
	GroupIndex    int
	Candidates    []int
	CandidateArgv [][]string
	Received      []string
}

func (e *GroupMismatchError) Error() string {
	return fmt.Sprintf("no match in group %q (index %d): received %v",
		e.GroupName, e.GroupIndex, e.Received)
}

// StdinMismatchError is returned when the command argv matches but stdin
// content does not match the expected value.
type StdinMismatchError struct {
	StepIndex int
	Expected  string
	Received  string
}

func (e *StdinMismatchError) Error() string {
	return fmt.Sprintf("stdin mismatch at step %d", e.StepIndex)
}

// ScenarioCompleteError is returned when all steps have been consumed.
type ScenarioCompleteError struct {
	TotalSteps int
}

func (e *ScenarioCompleteError) Error() string {
	return fmt.Sprintf("scenario already complete (all %d steps consumed)", e.TotalSteps)
}
